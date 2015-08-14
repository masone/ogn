package tsdb

import (
	"container/heap"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/influxdb/influxdb/influxql"
)

// MapperValue is a complex type, which can encapsulate data from both raw and aggregate
// mappers. This currently allows marshalling and network system to remain simpler. For
// aggregate output Time is ignored, and actual Time-Value pairs are contained soley
// within the Value field.
type MapperValue struct {
	Time  int64       `json:"time,omitempty"`  // Ignored for aggregate output.
	Value interface{} `json:"value,omitempty"` // For aggregate, contains interval time multiple values.
}

type MapperValues []*MapperValue

func (a MapperValues) Len() int           { return len(a) }
func (a MapperValues) Less(i, j int) bool { return a[i].Time < a[j].Time }
func (a MapperValues) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

type MapperOutput struct {
	Name   string            `json:"name,omitempty"`
	Tags   map[string]string `json:"tags,omitempty"`
	Fields []string          `json:"fields,omitempty"` // Field names of returned data.
	Values []*MapperValue    `json:"values,omitempty"` // For aggregates contains a single value at [0]
}

func (mo *MapperOutput) key() string {
	return formMeasurementTagSetKey(mo.Name, mo.Tags)
}

// LocalMapper is for retrieving data for a query, from a given shard.
type LocalMapper struct {
	shard           *Shard
	stmt            influxql.Statement
	selectStmt      *influxql.SelectStatement
	rawMode         bool
	chunkSize       int
	tx              Tx              // Read transaction for this shard.
	queryTMin       int64           // Minimum time of the query.
	queryTMax       int64           // Maximum time of the query.
	whereFields     []string        // field names that occur in the where clause
	selectFields    []string        // field names that occur in the select clause
	selectTags      []string        // tag keys that occur in the select clause
	cursors         []*tagSetCursor // Cursors per tag sets.
	currCursorIndex int             // Current tagset cursor being drained.

	// The following attributes are only used when mappers are for aggregate queries.

	queryTMinWindow int64              // Minimum time of the query floored to start of interval.
	intervalSize    int64              // Size of each interval.
	numIntervals    int                // Maximum number of intervals to return.
	currInterval    int                // Current interval for which data is being fetched.
	mapFuncs        []influxql.MapFunc // The mapping functions.
	fieldNames      []string           // the field name being read for mapping.
}

// NewLocalMapper returns a mapper for the given shard, which will return data for the SELECT statement.
func NewLocalMapper(shard *Shard, stmt influxql.Statement, chunkSize int) *LocalMapper {
	return &LocalMapper{
		shard:     shard,
		stmt:      stmt,
		chunkSize: chunkSize,
		cursors:   make([]*tagSetCursor, 0),
	}
}

// openMeta opens the mapper for a meta query.
func (lm *LocalMapper) openMeta() error {
	return errors.New("not implemented")
}

// Open opens the local mapper.
func (lm *LocalMapper) Open() error {
	var err error

	// Get a read-only transaction.
	tx, err := lm.shard.engine.Begin(false)
	if err != nil {
		return err
	}
	lm.tx = tx

	if s, ok := lm.stmt.(*influxql.SelectStatement); ok {
		stmt, err := lm.rewriteSelectStatement(s)
		if err != nil {
			return err
		}
		lm.selectStmt = stmt
		lm.rawMode = (s.IsRawQuery && !s.HasDistinct()) || s.IsSimpleDerivative()
	} else {
		return lm.openMeta()
	}

	// Set all time-related parameters on the mapper.
	lm.queryTMin, lm.queryTMax = influxql.TimeRangeAsEpochNano(lm.selectStmt.Condition)

	if !lm.rawMode {
		if err := lm.initializeMapFunctions(); err != nil {
			return err
		}

		// For GROUP BY time queries, limit the number of data points returned by the limit and offset
		d, err := lm.selectStmt.GroupByInterval()
		if err != nil {
			return err
		}
		lm.intervalSize = d.Nanoseconds()
		if lm.queryTMin == 0 || lm.intervalSize == 0 {
			lm.numIntervals = 1
			lm.intervalSize = lm.queryTMax - lm.queryTMin
		} else {
			intervalTop := lm.queryTMax/lm.intervalSize*lm.intervalSize + lm.intervalSize
			intervalBottom := lm.queryTMin / lm.intervalSize * lm.intervalSize
			lm.numIntervals = int((intervalTop - intervalBottom) / lm.intervalSize)
		}

		if lm.selectStmt.Limit > 0 || lm.selectStmt.Offset > 0 {
			// ensure that the offset isn't higher than the number of points we'd get
			if lm.selectStmt.Offset > lm.numIntervals {
				return nil
			}

			// Take the lesser of either the pre computed number of GROUP BY buckets that
			// will be in the result or the limit passed in by the user
			if lm.selectStmt.Limit < lm.numIntervals {
				lm.numIntervals = lm.selectStmt.Limit
			}
		}

		// If we are exceeding our MaxGroupByPoints error out
		if lm.numIntervals > MaxGroupByPoints {
			return errors.New("too many points in the group by interval. maybe you forgot to specify a where time clause?")
		}

		// Ensure that the start time for the results is on the start of the window.
		lm.queryTMinWindow = lm.queryTMin
		if lm.intervalSize > 0 && lm.numIntervals > 1 {
			lm.queryTMinWindow = lm.queryTMinWindow / lm.intervalSize * lm.intervalSize
		}
	}

	selectFields := newStringSet()
	selectTags := newStringSet()
	whereFields := newStringSet()

	// Create the TagSet cursors for the Mapper.
	for _, src := range lm.selectStmt.Sources {
		mm, ok := src.(*influxql.Measurement)
		if !ok {
			return fmt.Errorf("invalid source type: %#v", src)
		}

		m := lm.shard.index.Measurement(mm.Name)
		if m == nil {
			// This shard have never received data for the measurement. No Mapper
			// required.
			return nil
		}

		// Validate that ANY GROUP BY is not a field for thie measurement.
		if err := m.ValidateGroupBy(lm.selectStmt); err != nil {
			return err
		}

		// Create tagset cursors and determine various field types within SELECT statement.
		tsf, err := createTagSetsAndFields(m, lm.selectStmt)
		if err != nil {
			return err
		}
		tagSets := tsf.tagSets
		selectFields.add(tsf.selectFields...)
		selectTags.add(tsf.selectTags...)
		whereFields.add(tsf.whereFields...)

		// Validate that any GROUP BY is not on a field
		if err := m.ValidateGroupBy(lm.selectStmt); err != nil {
			return err
		}

		// SLIMIT and SOFFSET the unique series
		if lm.selectStmt.SLimit > 0 || lm.selectStmt.SOffset > 0 {
			if lm.selectStmt.SOffset > len(tagSets) {
				tagSets = nil
			} else {
				if lm.selectStmt.SOffset+lm.selectStmt.SLimit > len(tagSets) {
					lm.selectStmt.SLimit = len(tagSets) - lm.selectStmt.SOffset
				}

				tagSets = tagSets[lm.selectStmt.SOffset : lm.selectStmt.SOffset+lm.selectStmt.SLimit]
			}
		}

		// Create all cursors for reading the data from this shard.
		for _, t := range tagSets {
			cursors := []*seriesCursor{}

			for i, key := range t.SeriesKeys {
				c := lm.tx.Cursor(key)
				if c == nil {
					// No data exists for this key.
					continue
				}
				cm := newSeriesCursor(c, t.Filters[i])
				cursors = append(cursors, cm)
			}

			tsc := newTagSetCursor(m.Name, t.Tags, cursors, lm.shard.FieldCodec(m.Name))
			tsc.pointHeap = newPointHeap()
			//Prime the buffers.
			for i := 0; i < len(tsc.cursors); i++ {
				k, v := tsc.cursors[i].SeekTo(lm.queryTMin)
				if k == -1 {
					continue
				}
				p := &pointHeapItem{
					timestamp: k,
					value:     v,
					cursor:    tsc.cursors[i],
				}
				heap.Push(tsc.pointHeap, p)
			}
			lm.cursors = append(lm.cursors, tsc)
		}
		sort.Sort(tagSetCursors(lm.cursors))
	}

	lm.selectFields = selectFields.list()
	lm.selectTags = selectTags.list()
	lm.whereFields = whereFields.list()

	// If the query does not aggregate, then at least 1 SELECT field should be present.
	if lm.rawMode && len(lm.selectFields) == 0 {
		// None of the SELECT fields exist in this data. Wipe out all tagset cursors.
		lm.cursors = nil
	}

	return nil
}

func (lm *LocalMapper) NextChunk() (interface{}, error) {
	if lm.rawMode {
		return lm.nextChunkRaw()
	}
	return lm.nextChunkAgg()
}

// nextChunkRaw returns the next chunk of data. Data comes in the same order as the
// tags return by TagSets. A chunk never contains data for more than 1 tagset.
// If there is no more data for any tagset, nil will be returned.
func (lm *LocalMapper) nextChunkRaw() (interface{}, error) {
	var output *MapperOutput
	for {
		if lm.currCursorIndex == len(lm.cursors) {
			// All tagset cursors processed. NextChunk'ing complete.
			return nil, nil
		}
		cursor := lm.cursors[lm.currCursorIndex]

		k, v := cursor.Next(lm.queryTMin, lm.queryTMax, lm.selectFields, lm.whereFields)
		if v == nil {
			// Tagset cursor is empty, move to next one.
			lm.currCursorIndex++
			if output != nil {
				// There is data, so return it and continue when next called.
				return output, nil
			} else {
				// Just go straight to the next cursor.
				continue
			}
		}

		if output == nil {
			output = &MapperOutput{
				Name:   cursor.measurement,
				Tags:   cursor.tags,
				Fields: lm.selectFields,
			}
		}
		value := &MapperValue{Time: k, Value: v}
		output.Values = append(output.Values, value)
		if len(output.Values) == lm.chunkSize {
			return output, nil
		}
	}
}

// nextChunkAgg returns the next chunk of data, which is the next interval of data
// for the current tagset. Tagsets are always processed in the same order as that
// returned by AvailTagsSets(). When there is no more data for any tagset nil
// is returned.
func (lm *LocalMapper) nextChunkAgg() (interface{}, error) {
	var output *MapperOutput
	for {
		if lm.currCursorIndex == len(lm.cursors) {
			// All tagset cursors processed. NextChunk'ing complete.
			return nil, nil
		}
		tsc := lm.cursors[lm.currCursorIndex]
		tmin, tmax := lm.nextInterval()

		if tmin < 0 {
			// All intervals complete for this tagset. Move to the next tagset.
			lm.currInterval = 0
			lm.currCursorIndex++
			continue
		}

		// Prep the return data for this tagset. This will hold data for a single interval
		// for a single tagset.
		if output == nil {
			output = &MapperOutput{
				Name:   tsc.measurement,
				Tags:   tsc.tags,
				Fields: lm.selectFields,
				Values: make([]*MapperValue, 1),
			}
			// Aggregate values only use the first entry in the Values field. Set the time
			// to the start of the interval.
			output.Values[0] = &MapperValue{
				Time:  tmin,
				Value: make([]interface{}, 0)}
		}

		// Always clamp tmin. This can happen as bucket-times are bucketed to the nearest
		// interval, and this can be less than the times in the query.
		qmin := tmin
		if qmin < lm.queryTMin {
			qmin = lm.queryTMin
		}

		tsc.pointHeap = newPointHeap()
		for i := range lm.mapFuncs {
			// Prime the tagset cursor for the start of the interval. This is not ideal, as
			// it should really calculate the values all in 1 pass, but that would require
			// changes to the mapper functions, which can come later.
			// Prime the buffers.
			for i := 0; i < len(tsc.cursors); i++ {
				k, v := tsc.cursors[i].SeekTo(tmin)
				if k == -1 {
					continue
				}
				p := &pointHeapItem{
					timestamp: k,
					value:     v,
					cursor:    tsc.cursors[i],
				}
				heap.Push(tsc.pointHeap, p)
			}
			// Wrap the tagset cursor so it implements the mapping functions interface.
			f := func() (time int64, value interface{}) {
				return tsc.Next(qmin, tmax, []string{lm.fieldNames[i]}, lm.whereFields)
			}

			tagSetCursor := &aggTagSetCursor{
				nextFunc: f,
			}

			// Execute the map function which walks the entire interval, and aggregates
			// the result.
			values := output.Values[0].Value.([]interface{})
			output.Values[0].Value = append(values, lm.mapFuncs[i](tagSetCursor))
		}
		return output, nil
	}
}

// nextInterval returns the next interval for which to return data. If start is less than 0
// there are no more intervals.
func (lm *LocalMapper) nextInterval() (start, end int64) {
	t := lm.queryTMinWindow + int64(lm.currInterval+lm.selectStmt.Offset)*lm.intervalSize

	// Onto next interval.
	lm.currInterval++
	if t > lm.queryTMax || lm.currInterval > lm.numIntervals {
		start, end = -1, 1
	} else {
		start, end = t, t+lm.intervalSize
	}
	return
}

// initializeMapFunctions initialize the mapping functions for the mapper. This only applies
// to aggregate queries.
func (lm *LocalMapper) initializeMapFunctions() error {
	var err error
	// Set up each mapping function for this statement.
	aggregates := lm.selectStmt.FunctionCalls()
	lm.mapFuncs = make([]influxql.MapFunc, len(aggregates))
	lm.fieldNames = make([]string, len(lm.mapFuncs))
	for i, c := range aggregates {
		lm.mapFuncs[i], err = influxql.InitializeMapFunc(c)
		if err != nil {
			return err
		}

		// Check for calls like `derivative(lmean(value), 1d)`
		var nested *influxql.Call = c
		if fn, ok := c.Args[0].(*influxql.Call); ok {
			nested = fn
		}
		switch lit := nested.Args[0].(type) {
		case *influxql.VarRef:
			lm.fieldNames[i] = lit.Val
		case *influxql.Distinct:
			if c.Name != "count" {
				return fmt.Errorf("aggregate call didn't contain a field %s", c.String())
			}
			lm.fieldNames[i] = lit.Val
		default:
			return fmt.Errorf("aggregate call didn't contain a field %s", c.String())
		}
	}

	return nil
}

// rewriteSelectStatement performs any necessary query re-writing.
func (lm *LocalMapper) rewriteSelectStatement(stmt *influxql.SelectStatement) (*influxql.SelectStatement, error) {
	var err error
	// Expand regex expressions in the FROM clause.
	sources, err := lm.expandSources(stmt.Sources)
	if err != nil {
		return nil, err
	}
	stmt.Sources = sources
	// Expand wildcards in the fields or GROUP BY.
	stmt, err = lm.expandWildcards(stmt)
	if err != nil {
		return nil, err
	}
	stmt.RewriteDistinct()
	return stmt, nil
}

// expandWildcards returns a new SelectStatement with wildcards in the fields
// and/or GROUP BY expanded with actual field names.
func (lm *LocalMapper) expandWildcards(stmt *influxql.SelectStatement) (*influxql.SelectStatement, error) {
	// If there are no wildcards in the statement, return it as-is.
	if !stmt.HasWildcard() {
		return stmt, nil
	}
	// Use sets to avoid duplicate field names.
	fieldSet := map[string]struct{}{}
	dimensionSet := map[string]struct{}{}
	var fields influxql.Fields
	var dimensions influxql.Dimensions
	// Iterate measurements in the FROM clause getting the fields & dimensions for each.
	for _, src := range stmt.Sources {
		if m, ok := src.(*influxql.Measurement); ok {
			// Lookup the measurement in the database.
			mm := lm.shard.index.Measurement(m.Name)
			if mm == nil {
				// This shard have never received data for the measurement. No Mapper
				// required.
				return stmt, nil
			}
			// Get the fields for this measurement.
			for _, name := range mm.FieldNames() {
				if _, ok := fieldSet[name]; ok {
					continue
				}
				fieldSet[name] = struct{}{}
				fields = append(fields, &influxql.Field{Expr: &influxql.VarRef{Val: name}})
			}
			// Get the dimensions for this measurement.
			for _, t := range mm.TagKeys() {
				if _, ok := dimensionSet[t]; ok {
					continue
				}
				dimensionSet[t] = struct{}{}
				dimensions = append(dimensions, &influxql.Dimension{Expr: &influxql.VarRef{Val: t}})
			}
		}
	}
	// Return a new SelectStatement with the wild cards rewritten.
	return stmt.RewriteWildcards(fields, dimensions), nil
}

// expandSources expands regex sources and removes duplicates.
// NOTE: sources must be normalized (db and rp set) before calling this function.
func (lm *LocalMapper) expandSources(sources influxql.Sources) (influxql.Sources, error) {
	// Use a map as a set to prevent duplicates. Two regexes might produce
	// duplicates when expanded.
	set := map[string]influxql.Source{}
	names := []string{}
	// Iterate all sources, expanding regexes when they're found.
	for _, source := range sources {
		switch src := source.(type) {
		case *influxql.Measurement:
			if src.Regex == nil {
				name := src.String()
				set[name] = src
				names = append(names, name)
				continue
			}
			// Get measurements from the database that match the regex.
			measurements := lm.shard.index.measurementsByRegex(src.Regex.Val)
			// Add those measurements to the set.
			for _, m := range measurements {
				m2 := &influxql.Measurement{
					Database:        src.Database,
					RetentionPolicy: src.RetentionPolicy,
					Name:            m.Name,
				}
				name := m2.String()
				if _, ok := set[name]; !ok {
					set[name] = m2
					names = append(names, name)
				}
			}
		default:
			return nil, fmt.Errorf("expandSources: unsuported source type: %T", source)
		}
	}
	// Sort the list of source names.
	sort.Strings(names)
	// Convert set to a list of Sources.
	expanded := make(influxql.Sources, 0, len(set))
	for _, name := range names {
		expanded = append(expanded, set[name])
	}
	return expanded, nil
}

// TagSets returns the list of TagSets for which this mapper has data.
func (lm *LocalMapper) TagSets() []string {
	return tagSetCursors(lm.cursors).Keys()
}

// Fields returns any SELECT fields. If this Mapper is not processing a SELECT query
// then an empty slice is returned.
func (lm *LocalMapper) Fields() []string {
	return lm.selectFields
}

// Close closes the mapper.
func (lm *LocalMapper) Close() {
	if lm != nil && lm.tx != nil {
		_ = lm.tx.Rollback()
	}
}

// aggTagSetCursor wraps a standard tagSetCursor, such that the values it emits are aggregated
// by intervals.
type aggTagSetCursor struct {
	nextFunc func() (time int64, value interface{})
}

// Next returns the next value for the aggTagSetCursor. It implements the interface expected
// by the mapping functions.
func (a *aggTagSetCursor) Next() (time int64, value interface{}) {
	return a.nextFunc()
}

type pointHeapItem struct {
	timestamp int64
	value     []byte
	cursor    *seriesCursor // cursor whence pointHeapItem came
}

type pointHeap []*pointHeapItem

func newPointHeap() *pointHeap {
	q := make(pointHeap, 0)
	heap.Init(&q)
	return &q
}

func (pq pointHeap) Len() int { return len(pq) }

func (pq pointHeap) Less(i, j int) bool {
	// We want a min-heap (points in chronological order), so use less than.
	return pq[i].timestamp < pq[j].timestamp
}

func (pq pointHeap) Swap(i, j int) { pq[i], pq[j] = pq[j], pq[i] }

func (pq *pointHeap) Push(x interface{}) {
	item := x.(*pointHeapItem)
	*pq = append(*pq, item)
}

func (pq *pointHeap) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

// tagSetCursor is virtual cursor that iterates over mutiple series cursors, as though it were
// a single series.
type tagSetCursor struct {
	measurement string            // Measurement name
	tags        map[string]string // Tag key-value pairs
	cursors     []*seriesCursor   // Underlying series cursors.
	decoder     *FieldCodec       // decoder for the raw data bytes

	// pointHeap is a min-heap, ordered by timestamp, that contains the next
	// point from each seriesCursor. Queries sometimes pull points from
	// thousands of series. This makes it reasonably efficient to find the
	// point with the next lowest timestamp among the thousands of series that
	// the query is pulling points from.
	// Performance profiling shows that this lookahead needs to be part
	// of the tagSetCursor type and not part of the the cursors type.
	pointHeap *pointHeap
}

// tagSetCursors represents a sortable slice of tagSetCursors.
type tagSetCursors []*tagSetCursor

func (a tagSetCursors) Len() int           { return len(a) }
func (a tagSetCursors) Less(i, j int) bool { return a[i].key() < a[j].key() }
func (a tagSetCursors) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func (a tagSetCursors) Keys() []string {
	keys := []string{}
	for i := range a {
		keys = append(keys, a[i].key())
	}
	sort.Strings(keys)
	return keys
}

// newTagSetCursor returns a tagSetCursor
func newTagSetCursor(m string, t map[string]string, c []*seriesCursor, d *FieldCodec) *tagSetCursor {
	tsc := &tagSetCursor{
		measurement: m,
		tags:        t,
		cursors:     c,
		decoder:     d,
		pointHeap:   newPointHeap(),
	}

	return tsc
}

func (tsc *tagSetCursor) key() string {
	return formMeasurementTagSetKey(tsc.measurement, tsc.tags)
}

// Next returns the next matching series-key, timestamp and byte slice for the tagset. Filtering
// is enforced on the values. If there is no matching value, then a nil result is returned.
func (tsc *tagSetCursor) Next(tmin, tmax int64, selectFields, whereFields []string) (int64, interface{}) {
	for {
		// If we're out of points, we're done.
		if tsc.pointHeap.Len() == 0 {
			return -1, nil
		}

		// Grab the next point with the lowest timestamp.
		p := heap.Pop(tsc.pointHeap).(*pointHeapItem)

		// We're done if the point is outside the query's time range [tmin:tmax).
		if p.timestamp != tmin && (tmin > p.timestamp || p.timestamp >= tmax) {
			return -1, nil
		}

		// Advance the cursor
		nextKey, nextVal := p.cursor.Next()
		if nextKey != -1 {
			nextPoint := &pointHeapItem{
				timestamp: nextKey,
				value:     nextVal,
				cursor:    p.cursor,
			}
			heap.Push(tsc.pointHeap, nextPoint)
		}

		// Decode the raw point.
		value := tsc.decodeRawPoint(p, selectFields, whereFields)

		// Value didn't match, look for the next one.
		if value == nil {
			continue
		}

		return p.timestamp, value
	}
}

// decodeRawPoint decodes raw point data into field names & values and does WHERE filtering.
func (tsc *tagSetCursor) decodeRawPoint(p *pointHeapItem, selectFields, whereFields []string) interface{} {
	if len(selectFields) > 1 {
		if fieldsWithNames, err := tsc.decoder.DecodeFieldsWithNames(p.value); err == nil {
			// if there's a where clause, make sure we don't need to filter this value
			if p.cursor.filter != nil && !matchesWhere(p.cursor.filter, fieldsWithNames) {
				return nil
			}

			return fieldsWithNames
		}
	}

	// With only 1 field SELECTed, decoding all fields may be avoidable, which is faster.
	value, err := tsc.decoder.DecodeByName(selectFields[0], p.value)
	if err != nil {
		return nil
	}

	// If there's a WHERE clase, see if we need to filter
	if p.cursor.filter != nil {
		// See if the WHERE is only on this field or on one or more other fields.
		// If the latter, we'll have to decode everything
		if len(whereFields) == 1 && whereFields[0] == selectFields[0] {
			if !matchesWhere(p.cursor.filter, map[string]interface{}{selectFields[0]: value}) {
				value = nil
			}
		} else { // Decode everything
			fieldsWithNames, err := tsc.decoder.DecodeFieldsWithNames(p.value)
			if err != nil || !matchesWhere(p.cursor.filter, fieldsWithNames) {
				value = nil
			}
		}
	}

	return value
}

// seriesCursor is a cursor that walks a single series. It provides lookahead functionality.
type seriesCursor struct {
	cursor Cursor // BoltDB cursor for a series
	filter influxql.Expr
}

// newSeriesCursor returns a new instance of a series cursor.
func newSeriesCursor(cur Cursor, filter influxql.Expr) *seriesCursor {
	return &seriesCursor{
		cursor: cur,
		filter: filter,
	}
}

// Seek positions returning the timestamp and value at that key.
func (sc *seriesCursor) SeekTo(key int64) (timestamp int64, value []byte) {
	k, v := sc.cursor.Seek(u64tob(uint64(key)))
	if k == nil {
		timestamp = -1
	} else {
		timestamp, value = int64(btou64(k)), v
	}
	return
}

// Next returns the next timestamp and value from the cursor.
func (sc *seriesCursor) Next() (key int64, value []byte) {
	k, v := sc.cursor.Next()
	if k == nil {
		key = -1
	} else {
		key, value = int64(btou64(k)), v
	}
	return
}

type tagSetsAndFields struct {
	tagSets      []*influxql.TagSet
	selectFields []string
	selectTags   []string
	whereFields  []string
}

// createTagSetsAndFields returns the tagsets and various fields given a measurement and
// SELECT statement.
func createTagSetsAndFields(m *Measurement, stmt *influxql.SelectStatement) (*tagSetsAndFields, error) {
	_, tagKeys, err := stmt.Dimensions.Normalize()
	if err != nil {
		return nil, err
	}

	sfs := newStringSet()
	sts := newStringSet()
	wfs := newStringSet()

	// Validate the fields and tags asked for exist and keep track of which are in the select vs the where
	for _, n := range stmt.NamesInSelect() {
		if m.HasField(n) {
			sfs.add(n)
			continue
		}
		if m.HasTagKey(n) {
			sts.add(n)
			tagKeys = append(tagKeys, n)
		}
	}
	for _, n := range stmt.NamesInWhere() {
		if n == "time" {
			continue
		}
		if m.HasField(n) {
			wfs.add(n)
			continue
		}
	}

	// Get the sorted unique tag sets for this statement.
	tagSets, err := m.TagSets(stmt, tagKeys)
	if err != nil {
		return nil, err
	}

	return &tagSetsAndFields{
		tagSets:      tagSets,
		selectFields: sfs.list(),
		selectTags:   sts.list(),
		whereFields:  wfs.list(),
	}, nil
}

// matchesFilter returns true if the value matches the where clause
func matchesWhere(f influxql.Expr, fields map[string]interface{}) bool {
	if ok, _ := influxql.Eval(f, fields).(bool); !ok {
		return false
	}
	return true
}

func formMeasurementTagSetKey(name string, tags map[string]string) string {
	if len(tags) == 0 {
		return name
	}
	return strings.Join([]string{name, string(MarshalTags(tags))}, "|")
}

// btou64 converts an 8-byte slice into an uint64.
func btou64(b []byte) uint64 { return binary.BigEndian.Uint64(b) }
