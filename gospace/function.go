// Package function is the Google Cloud Functions Gen 2 entry package for the
// generic gospace worker. The Functions buildpack owns main(); this package
// exposes an application-empty runtime. Composed deployments should build their
// own entry package with server.New(nativeHandler).
package function

import (
	"net/http"

	"github.com/dash-xd/gospace/server"
)

var defaultServer = server.New(nil)

var Main func(http.ResponseWriter, *http.Request) = defaultServer.ServeHTTP
