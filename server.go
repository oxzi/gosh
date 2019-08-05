package main

import (
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

type Server struct {
	store   *Store
	maxSize int64
}

func NewServer(storeDirectory string, maxSize int64) (s *Server, err error) {
	store, storeErr := NewStore(storeDirectory)
	if storeErr != nil {
		err = storeErr
		return
	}

	s = &Server{
		store:   store,
		maxSize: maxSize,
	}
	return
}

func (serv *Server) Close() error {
	return serv.store.Close()
}

func (serv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if reqPath := r.URL.Path; reqPath == "/" {
		serv.handleRoot(w, r)
	} else if reqPath[:3] == "/r/" {
		serv.handleRequest(w, r)
	} else {
		log.WithField("path", reqPath).Debug("Request to an unsupported path")

		http.Error(w, "Does not exists.", http.StatusNotFound)
	}
}

func (serv *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		serv.handleIndex(w, r)

	case "POST":
		serv.handleUpload(w, r)

	default:
		log.WithField("method", r.Method).Debug("Called with unsupported method")

		http.Error(w, "Method not supported.", http.StatusMethodNotAllowed)
	}
}

func (serv *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// TODO
	http.Error(w, "not supported yet", http.StatusTeapot)
}

func (serv *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	item, f, err := NewItem(r, serv.maxSize)
	if err != nil {
		log.WithError(err).Warn("Failed to create new Item")

		http.Error(w, "Something went wrong.", http.StatusBadRequest)
		return
	}

	item.Expires = time.Now().Add(30 * time.Second).UTC()

	itemId, err := serv.store.Put(item, f)
	if err != nil {
		log.WithError(err).Warn("Failed to store Item")

		http.Error(w, "Something went wrong.", http.StatusBadRequest)
		return
	}

	log.WithField("ID", itemId).Info("Uploaded new Item")

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s\n", itemId)
}

func (serv *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		log.WithField("method", r.Method).Debug("Request got wrong method")

		http.Error(w, "Method not supported", http.StatusMethodNotAllowed)
		return
	}

	reqId := r.URL.RequestURI()[3:]

	item, err := serv.store.Get(reqId)
	if err == ErrNotFound {
		log.WithField("ID", reqId).Debug("Requested non-existing ID")

		http.Error(w, "Does not exists.", http.StatusNotFound)
		return
	} else if err != nil {
		log.WithError(err).WithField("ID", reqId).Warn("Requesting errored")

		http.Error(w, "Something went wrong.", http.StatusBadRequest)
		return
	}

	if f, err := serv.store.GetFile(item); err != nil {
		log.WithError(err).WithField("ID", item.ID).Warn("Reading file errored")

		http.Error(w, "Something went wrong.", http.StatusBadRequest)
		return
	} else {
		w.Header().Set("Content-Type", item.ContentType)
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename=\"%s\"", item.Filename))
		w.WriteHeader(http.StatusOK)

		if _, err := io.Copy(w, f); err != nil {
			log.WithError(err).WithField("ID", item.ID).Warn("Writing file errored")
			return
		}

		log.WithField("ID", item.ID).Debug("Item was requested")
	}
}

func main() {
	server, err := NewServer("store", 10*1024*1024)
	if err != nil {
		log.WithError(err).Fatal("Failed to start Store")
	}

	http.ListenAndServe(":8080", server)

	if err := server.Close(); err != nil {
		log.WithError(err).Fatal("Closing errored")
	}
}
