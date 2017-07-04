package exporter_common

import (
	"crypto/subtle"
	"flag"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
	"gopkg.in/yaml.v2"
)

var (
	authFileF = flag.String(
		"web.auth-file", "",
		"Path to YAML file with server_user, server_password keys for HTTP Basic authentication "+
			"(overrides HTTP_AUTH environment variable).",
	)
)

// basicAuth combines username and password.
type basicAuth struct {
	Username string `yaml:"server_user,omitempty"`
	Password string `yaml:"server_password,omitempty"`
}

// readBasicAuth returns basicAuth from -web.auth-file file, or HTTP_AUTH environment variable, or empty one.
func readBasicAuth() *basicAuth {
	var auth basicAuth
	httpAuth := os.Getenv("HTTP_AUTH")
	switch {
	case *authFileF != "":
		bytes, err := ioutil.ReadFile(*authFileF)
		if err != nil {
			log.Fatalf("cannot read auth file %q: %s", *authFileF, err)
		}
		if err = yaml.Unmarshal(bytes, &auth); err != nil {
			log.Fatalf("cannot parse auth file %q: %s", *authFileF, err)
		}
	case httpAuth != "":
		data := strings.SplitN(httpAuth, ":", 2)
		if len(data) != 2 || data[0] == "" || data[1] == "" {
			log.Fatalf("HTTP_AUTH should be formatted as user:password")
		}
		auth.Username = data[0]
		auth.Password = data[1]
	default:
		// that's fine, return empty one below
	}

	return &auth
}

// basicAuthHandler checks username and password before invoking provided handler.
type basicAuthHandler struct {
	basicAuth
	handler http.HandlerFunc
}

// check interface
var _ http.Handler = (*basicAuthHandler)(nil)

// ServeHTTP implements http.Handler.
func (h *basicAuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	username, password, _ := r.BasicAuth()
	usernameOk := subtle.ConstantTimeCompare([]byte(h.Username), []byte(username)) == 1
	passwordOk := subtle.ConstantTimeCompare([]byte(h.Password), []byte(password)) == 1
	if !usernameOk || !passwordOk {
		w.Header().Set("WWW-Authenticate", `Basic realm="metrics"`)
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	h.handler(w, r)
}

// handler returns http.Handler for default Prometheus registry.
func handler() http.Handler {
	handler := promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{
		ErrorLog:      log.NewErrorLogger(),
		ErrorHandling: promhttp.HTTPErrorOnError,
	})

	auth := readBasicAuth()
	if auth.Username != "" && auth.Password != "" {
		handler = &basicAuthHandler{basicAuth: *auth, handler: handler.ServeHTTP}
		log.Infoln("HTTP Basic authentication is enabled.")
	}

	return handler
}