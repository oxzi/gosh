package main

import (
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

type Server struct {
	store *Store
}

const (
	dirDb    = "store/db"
	dirFiles = "store/files"

	maxSize = 10 * 1024 * 1024
)

func (serv *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		serv.handleRootGet(w, r)

	case "POST":
		serv.handleRootPost(w, r)

	default:
		log.WithField("method", r.Method).Info("Called with unsupported method")

		http.Error(w, "Method not supported", http.StatusMethodNotAllowed)
	}
}

func (serv *Server) handleRootPost(w http.ResponseWriter, r *http.Request) {
	item, _, err := NewItem(r, maxSize)
	if err != nil {
		log.WithError(err).Warn("Failed to create new Item")

		http.Error(w, "", http.StatusBadRequest)
		return
	}

	// TODO: original/better ID
	item.ID = fmt.Sprintf("%d", time.Now().UTC().Unix())
	item.Expires = time.Now().Add(10 * time.Second).UTC()

	if err := serv.store.Put(item); err != nil {
		log.WithError(err).Warn("Failed to store Item")

		http.Error(w, "", http.StatusBadRequest)
		return
	}

	// TODO: write file to disk

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s", item.ID)
}

func (serv *Server) handleRootGet(w http.ResponseWriter, r *http.Request) {
	// TODO
	http.Error(w, "not supported yet", http.StatusTeapot)
}

func (serv *Server) handleFileReq(w http.ResponseWriter, r *http.Request) {
	reqId := r.URL.RequestURI()[3:]

	item, err := serv.store.Get(reqId)
	if err == ErrNotFound {
		log.WithField("ID", reqId).Info("Requested non-existing ID")

		http.Error(w, "not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.WithError(err).WithField("ID", reqId).Warn("Requesting errored")

		http.Error(w, "", http.StatusBadRequest)
		return
	}

	log.Info(item)

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "okäse: %s", item.ID)
}

func main() {
	store, err := NewStore(dirDb, dirFiles)
	if err != nil {
		log.WithError(err).Fatal("Failed to start Store")
	}

	serv := &Server{store: store}

	mux := http.NewServeMux()
	mux.HandleFunc("/", serv.handleRoot)
	mux.HandleFunc("/r/", serv.handleFileReq)

	http.ListenAndServe(":8080", mux)

	store.Close()
}
