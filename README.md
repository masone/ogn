## Purpose

Automated detection of starts and landings on a defined airfield by processing the APRS traffic from the (Open Glider Network OGN)[http://glidernet.org].
Work in progress.


## Features

- Detection of starts and landings
- Calculation of total flight time
- Detection of launch type (winch, tow, self start)


## Notes

Here's a few things you should know.

### Coverage

The (coverage tool)[http://ognrange.onglide.com/] displays the theoretical range of receivers.
Make sure your airfield is covered by a nearby receiver and not in a dead spot.

### Tracked aircrafts

This tool only tracks aircrafts which are FLARM equipped and found in the (OGN devices database)[http://ddb.glidernet.org/].
Yes, this also includes your tow plane.

### Accuracy

Please note that the detected start and landing times are approximate only. Due to several technical reasons,
you have to expect a small offset from the real start time. In my experience, they're no more than 1-2 minutes off.

### Default constants

There are a few constants in the code. They work very well for the airfield I am tracking
but might need some tweaking if yours is substantially different. The same goes for
the detection of the start type. The constants work well for our tow plane type and winch cord length.


## Installation

1. Install [libfap](http://www.pakettiradio.net/libfap/)
2. Compile binary with Go
```
go build && ./ogn
```

You can also read APRS data from a logfile
```
go build && ./ogn ogn.2015-08-28.log
```

Global logfiles can get pretty big. You can lower the size by filtering by a nearby receiver:
```
grep -e 'LSPH' ogn.2015-08-28.log > lsph.2015-08-28.log
```

## Deploy to Heroku

Create a new app with postgres activated and install the buildkits plugin

```
heroku create your_tracker
heroku addons:create heroku-postgresql
heroku plugins:install https://github.com/heroku/heroku-buildkits
heroku buildkits:set https://github.com/ddollar/heroku-buildpack-multi.git
```

Configure your new app as follows

```
heroku config:set CGO_CFLAGS=-I/app/.dpkg/usr/include
heroku config:set CGO_LDFLAGS=-L/app/.dpkg/usr/lib

heroku config:set APRS_USER=ogn123     # a random user identification
heroku config:set APRS_RADIUS=100      # km from the airfield to still track positions

heroku config:set AF_LAT=46.8333       # lat of the airfield to track
heroku config:set AF_LNG=8.3333        # lng of the airfield to track
heroku config:set AF_ELEVATION=470     # elevation of the airfield to track
```

In the webinterface, spin up a `tracker` dyno under `Resources`.
