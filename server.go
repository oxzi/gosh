package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

type Server struct {
	store       *Store
	maxSize     int64
	maxLifetime time.Duration
}

func NewServer(storeDirectory string, maxSize int64, maxLifetime time.Duration) (s *Server, err error) {
	store, storeErr := NewStore(storeDirectory)
	if storeErr != nil {
		err = storeErr
		return
	}

	s = &Server{
		store:       store,
		maxSize:     maxSize,
		maxLifetime: maxLifetime,
	}
	return
}

func (serv *Server) Close() error {
	return serv.store.Close()
}

func (serv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if reqPath := r.URL.Path; reqPath == "/" {
		serv.handleRoot(w, r)
	} else {
		serv.handleRequest(w, r)
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
	http.Error(w, "not supported yet", http.StatusTeapot)
}

func (serv *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	item, f, err := NewItem(r, serv.maxSize, serv.maxLifetime)
	if err == ErrLifetimeToLong {
		log.Info("New Item with a too great lifetime was rejected")

		http.Error(w, "Lifetime exceeds maximum", http.StatusNotAcceptable)
		return
	} else if err != nil {
		log.WithError(err).Warn("Failed to create new Item")

		http.Error(w, "Something went wrong.", http.StatusBadRequest)
		return
	}

	itemId, err := serv.store.Put(item, f)
	if err != nil {
		log.WithError(err).Warn("Failed to store Item")

		http.Error(w, "Something went wrong.", http.StatusBadRequest)
		return
	}

	log.WithFields(log.Fields{
		"ID":       itemId,
		"filename": item.Filename,
		"expires":  item.Expires,
	}).Info("Uploaded new Item")

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "http://%s/%s\n", r.Host, itemId)
}

func (serv *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		log.WithField("method", r.Method).Debug("Request got wrong method")

		http.Error(w, "Method not supported", http.StatusMethodNotAllowed)
		return
	}

	reqId := strings.TrimLeft(r.URL.Path, "/")

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
			// This might happen if the peer resets the connection, e.g., if
			// curl tries to print a non text file to stdout.
			log.WithError(err).WithField("ID", item.ID).Warn("Writing file errored")
		}

		log.WithFields(log.Fields{
			"ID":       item.ID,
			"filename": item.Filename,
		}).Info("Item was requested")
	}

	if item.BurnAfterReading {
		log.WithField("ID", item.ID).Info("Item will be burned")
		if err := serv.store.Delete(item); err != nil {
			log.WithError(err).WithField("ID", item.ID).Warn("Deletion errored")
		}
	}
}

func main() {
	server, err := NewServer("store", 10*1024*1024, 10*time.Minute)
	if err != nil {
		log.WithError(err).Fatal("Failed to start Store")
	}

	http.ListenAndServe(":8080", server)

	if err := server.Close(); err != nil {
		log.WithError(err).Fatal("Closing errored")
	}
}
