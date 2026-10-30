package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

const (
	// readyzTimeout is a backstop; the ping's driver timeouts end it earlier.
	readyzTimeout = 5 * time.Second
	// readyzReuse lets concurrent and closely following /readyz calls share one ping.
	readyzReuse = time.Second
)

// InitHealthRouter registers the Kubernetes probe endpoints. Both are
// unauthenticated on purpose and answer only "ok" or "unavailable".
// ping checks the database on a dedicated connection (database.Ping).
func InitHealthRouter(r *mux.Router, ping func(context.Context) error) *mux.Router {
	rd := &readiness{ping: ping}
	r.HandleFunc("/healthz", healthz).Methods("GET")
	r.HandleFunc("/readyz", rd.readyz).Methods("GET")
	return r
}

// healthz is the liveness probe: no dependency checks, so a database outage
// never makes Kubernetes restart the pod.
func healthz(w http.ResponseWriter, _ *http.Request) {
	writeProbe(w, http.StatusOK, "ok")
}

// readiness runs at most one database ping at a time; callers arriving while a
// ping runs, or within readyzReuse after it, get its result.
type readiness struct {
	ping func(context.Context) error

	mu  sync.Mutex
	at  time.Time
	err error
}

func (rd *readiness) check() error {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	if !rd.at.IsZero() && time.Since(rd.at) < readyzReuse {
		return rd.err
	}
	// Not the request context: the result is shared, one caller leaving must not fail the others.
	ctx, cancel := context.WithTimeout(context.Background(), readyzTimeout)
	defer cancel()
	rd.err = rd.ping(ctx)
	rd.at = time.Now()
	if rd.err != nil {
		log.Warnf("readyz: database not ready: %v", rd.err)
	}
	return rd.err
}

// readyz is the readiness probe: the database must answer a trivial query.
func (rd *readiness) readyz(w http.ResponseWriter, _ *http.Request) {
	if rd.check() != nil {
		writeProbe(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeProbe(w, http.StatusOK, "ok")
}

func writeProbe(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
