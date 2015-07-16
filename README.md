## Purpose

Automated detection of starts and landings on a defined airfield by processing the APRS traffic from the [Open Glider Network OGN](glidernet.org).

## Installation

1. Install [libfap](http://www.pakettiradio.net/libfap/)
2. Compile binary with Go
```
go build && ./ogn
```

## Deploy to Heroku

Create a new app and install the buildkits plugin

```
heroku create your_tracker
heroku plugins:install https://github.com/heroku/heroku-buildkits
heroku buildkits:set https://github.com/ddollar/heroku-buildpack-multi.git
```

Configure your new app in the web interface or console like follows

```
heroku config:set CGO_CFLAGS=-I/app/.dpkg/usr/include
heroku config:set CGO_LDFLAGS=-L/app/.dpkg/usr/lib

heroku config:set INFLUX_DATABASE=database
heroku config:set INFLUX_HOST=yourhost.com
heroku config:set INFLUX_PORT=8086
heroku config:set INFLUX_USERNAME=username
heroku config:set INFLUX_PASSWORD=password
```

In the webinterface, spin up a `tracker` dyno under `Resources`.
