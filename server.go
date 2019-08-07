package gosh

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
	"time"

	log "github.com/sirupsen/logrus"
)

const indexTpl = `# gosh! Go Share

Upload your files to this server and share them with your friends or, if
non-existent, shady people from the Internet. Your file will expire after
{{.Expires}} or earlier, if explicitly specified. Optionally, the file can be
deleted directly after the first retrieval. In addition, the maximum
file size is {{.Size}}.

This is no place to share questionable or illegal data. Please use another
service or stop it completely. Get some help.

The gosh software can be obtained from <https://github.com/geistesk/gosh>.


## Posting

HTTP POST your file:
$ curl -F 'file=@foo.png' {{.Proto}}://{{.Hostname}}/

Burn after reading:
$ curl -F 'file=@foo.png' -F 'burn=1' {{.Proto}}://{{.Hostname}}/

Set a custom expiry date, e.g., one minute:
$ curl -F 'file=@foo.png' -F 'time=1m' {{.Proto}}://{{.Hostname}}/

Or all together:
$ curl -F 'file=@foo.png' -F 'time=1m' -F 'burn=1' {{.Proto}}://{{.Hostname}}/


## Privacy

This software stores the IP address for each upload. This information is
stored as long as the file is available. In addition, the IP address of the
user might be loged in case of an error. A normal download is logged without
user information.


## Abuse

If, for whatever reason, you would like to have a file removed prematurely,
please write an e-mail to <{{.EMail}}>.

Please allow me a certain amount of time to react and work on your request.
`

const (
	msgFileSizeExceeds   = "Error: File size exceeds maximum."
	msgGenericError      = "Error: Something went wrong."
	msgIllegalMime       = "Error: MIME type is blacklisted."
	msgLifetimeExceeds   = "Error: Lifetime exceeds maximum."
	msgNotExists         = "Error: Does not exists."
	msgUnsupportedMethod = "Error: Method not supported."
)

// Server implements an http.Handler for up- and download.
type Server struct {
	store       *Store
	maxSize     int64
	maxLifetime time.Duration
	contactMail string
	mimeMap     MimeMap
}

// NewServer creates a new Server with a given database directory, and
// configuration values. The Server must be started as an http.Handler.
func NewServer(storeDirectory string, maxSize int64, maxLifetime time.Duration,
	contactMail string, mimeMap MimeMap) (s *Server, err error) {
	store, storeErr := NewStore(storeDirectory, true)
	if storeErr != nil {
		err = storeErr
		return
	}

	s = &Server{
		store:       store,
		maxSize:     maxSize,
		maxLifetime: maxLifetime,
		contactMail: contactMail,
		mimeMap:     mimeMap,
	}
	return
}

// Close the Server and its components.
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

		http.Error(w, msgUnsupportedMethod, http.StatusMethodNotAllowed)
	}
}

func (serv *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	t, err := template.New("index").Parse(indexTpl)
	if err != nil {
		log.WithError(err).Warn("Failed to parse template")

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	}

	data := struct {
		Expires  string
		Size     string
		Proto    string
		Hostname string
		EMail    string
	}{
		Expires:  PrettyDuration(serv.maxLifetime),
		Size:     PrettyBytesize(serv.maxSize),
		Proto:    WebProtocol(r),
		Hostname: r.Host,
		EMail:    serv.contactMail,
	}

	w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
	w.WriteHeader(http.StatusOK)

	if err := t.Execute(w, data); err != nil {
		log.WithError(err).Warn("Failed to execute template")
	}
}

func (serv *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	item, f, err := NewItem(r, serv.maxSize, serv.maxLifetime)
	if err == ErrLifetimeToLong {
		log.Info("New Item with a too great lifetime was rejected")

		http.Error(w, msgLifetimeExceeds, http.StatusNotAcceptable)
		return
	} else if err == ErrFileToBig {
		log.Info("New Item with a too great file size was rejected")

		http.Error(w, msgFileSizeExceeds, http.StatusNotAcceptable)
		return
	} else if err != nil {
		log.WithError(err).Warn("Failed to create new Item")

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	} else if serv.mimeMap.MustDrop(item.ContentType) {
		log.WithField("MIME", item.ContentType).Info("Prevented upload of an illegal MIME")

		http.Error(w, msgIllegalMime, http.StatusBadRequest)
		return
	}

	itemId, err := serv.store.Put(item, f)
	if err != nil {
		log.WithError(err).Warn("Failed to store Item")

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	}

	log.WithFields(log.Fields{
		"ID":       itemId,
		"filename": item.Filename,
		"expires":  item.Expires,
	}).Info("Uploaded new Item")

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s://%s/%s\n", WebProtocol(r), r.Host, itemId)
}

func (serv *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		log.WithField("method", r.Method).Debug("Request got wrong method")

		http.Error(w, msgUnsupportedMethod, http.StatusMethodNotAllowed)
		return
	}

	reqId := strings.TrimLeft(r.URL.Path, "/")

	item, err := serv.store.Get(reqId, true)
	if err == ErrNotFound {
		log.WithField("ID", reqId).Debug("Requested non-existing ID")

		http.Error(w, msgNotExists, http.StatusNotFound)
		return
	} else if err != nil {
		log.WithError(err).WithField("ID", reqId).Warn("Requesting errored")

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	}

	if f, err := serv.store.GetFile(item); err != nil {
		log.WithError(err).WithField("ID", item.ID).Warn("Reading file errored")

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	} else {
		mimeType, mimeErr := serv.mimeMap.Substitute(item.ContentType)
		if mimeErr != nil {
			log.WithError(err).WithField("ID", item.ID).Warn("Substituting MIME errored")

			http.Error(w, msgGenericError, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("inline; filename=\"%s\"", item.Filename))
		w.WriteHeader(http.StatusOK)

		if _, err := io.Copy(w, f); err != nil {
			// This might happen if the peer resets the connection, e.g., if
			// curl tries to print a non text file to stdout.
			log.WithError(err).WithField("ID", item.ID).Warn("Writing file errored")
		}

		if err := f.Close(); err != nil {
			log.WithError(err).WithField("ID", item.ID).Warn("Closing file errored")
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

// WebProtocol returns "http" or "https", based on the X-Forwarded-Proto header.
func WebProtocol(r *http.Request) string {
	if xfwp := r.Header.Get("X-Forwarded-Proto"); xfwp != "" {
		return xfwp
	} else {
		return "http"
	}
}
