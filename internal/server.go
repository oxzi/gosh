package internal

import (
	"fmt"
	"io"
	"net/http"
	"net/http/fcgi"
	"strings"
	"text/template"
	"time"

	log "github.com/sirupsen/logrus"
)

const indexTpl = `<!DOCTYPE html>
<html>
	<head>
		<title>gosh! Go Share</title>

		<style>
			* {
				font-family: monospace;
			}

			body {
				margin: 0 auto;
				padding: 1rem;
				width: 50%;
			}

			h1 {
				padding-top: 3rem;
			}

			h2 {
				padding-top: 2rem;
			}

			h3 {
				padding-top: 1rem;
			}

			pre {
				background-color: #eee;
				padding: 0.5rem;
			}

			form {
				padding: 0.5rem;
				position: relative;
				margin: auto;
				background-color: #eee;
			}

			#grid {
				display: grid;
				grid-gap: 1rem;
				grid-template-columns: 1fr 1fr;
				grid-template-rows: repeat(3, 3rem);
				margin-bottom: 1rem;
			}

			#grid > * {
				margin: auto 0;
			}

			#grid input[type="checkbox"] {
				margin-right: auto;
			}

			button {
				width: 100%;
			}
		</style>
	</head>

	<body>
		<h1># gosh! Go Share</h1>
		<p>
			Upload your files to this server and share them with your friends or, if
			non-existent, shady people from the Internet.
		</p>
		<p>
			Your file will expire after {{.Expires}} or earlier, if explicitly
			specified. Optionally, the file can be deleted directly after the first
			retrieval. For each upload, a deletion URL will also be generated which
			can be used to delete the file before expiration. In addition, the
			maximum file size is {{.Size}}.
		</p>
		<p>
			This is no place to share questionable or illegal data. Please use another
			service or stop it completely. Get some help.
		</p>
		<p>
			The gosh software can be obtained from
			<a href="https://github.com/oxzi/gosh">https://github.com/oxzi/gosh</a>
		</p>

		<h2>## Posting</h2>

		<h3>### curl</h3>

		HTTP POST your file:

		<pre>$ curl -F 'file=@foo.png' {{.Proto}}://{{.Hostname}}/</pre>

		Burn after reading:

		<pre>$ curl -F 'file=@foo.png' -F 'burn=1' {{.Proto}}://{{.Hostname}}/</pre>

		Set a custom expiry date, e.g., one minute:

		<pre>$ curl -F 'file=@foo.png' -F 'time=1m' {{.Proto}}://{{.Hostname}}/</pre>

		Or all together:

		<pre>$ curl -F 'file=@foo.png' -F 'time=1m' -F 'burn=1' {{.Proto}}://{{.Hostname}}/</pre>

		Print only URL as response:

		<pre>$ curl -F 'file=@foo.png' -F {{.Proto}}://{{.Hostname}}/?onlyURL</pre>

		<h3>### form</h3>

		<form
			action="{{.Proto}}://{{.Hostname}}/"
			method="POST"
			enctype="multipart/form-data">
			<div id="grid">
				<label for="file">Your file:</label>
				<input type="file" name="file" />
				<label for="burn">Burn after reading:</label>
				<input type="checkbox" name="burn" value="1" />
				<label for="time">Optionally, set a custom expiry date:</label>
				<input
					type="text"
					name="time"
					pattern="{{.DurationPattern}}"
					title="A duration string is sequence of decimal numbers, each with a unit suffix. Valid time units in order are 'y', 'mo', 'w', 'd', 'h', 'm', 's'"
				/>
			</div>
			<button>Upload</button>
		</form>

		<h2>## Privacy</h2>

		This software stores the IP address for each upload. This information is
		stored as long as the file is available. A normal download is logged without
		user information.

		<h2>## Abuse</h2>

		If, for whatever reason, you would like to have a file removed prematurely,
		please write an e-mail to
		<a href="mailto:{{.EMail}}">&lt;{{.EMail}}&gt;</a>. Please allow me a
		certain amount of time to react and work on your request.
	</body>
</html>
`

const (
	msgDeletionKeyWrong  = "Error: Deletion key is incorrect."
	msgDeletionSuccess   = "OK: Item was deleted."
	msgFileSizeExceeds   = "Error: File size exceeds maximum."
	msgGenericError      = "Error: Something went wrong."
	msgIllegalMime       = "Error: MIME type is blacklisted."
	msgLifetimeExceeds   = "Error: Lifetime exceeds maximum."
	msgNotExists         = "Error: Does not exist."
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
	reqPath := r.URL.Path
	if reqPath == "/" {
		serv.handleRoot(w, r)
	} else if strings.HasPrefix(reqPath, "/del/") {
		serv.handleDeletion(w, r)
	} else {
		serv.handleRequest(w, r)
	}
}

func (serv *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		serv.handleIndex(w, r)

	case http.MethodPost:
		serv.handleUpload(w, r)

	default:
		log.WithField("method", r.Method).Debug("Called with unsupported method")

		http.Error(w, msgUnsupportedMethod, http.StatusMethodNotAllowed)
	}
}

func (serv *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	t, err := template.New("index").Parse(indexTpl)
	if err != nil {
		log.WithError(err).Error("Failed to parse template")

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	}

	data := struct {
		Expires         string
		Size            string
		Proto           string
		Hostname        string
		EMail           string
		DurationPattern string
	}{
		Expires:         PrettyDuration(serv.maxLifetime),
		Size:            PrettyBytesize(serv.maxSize),
		Proto:           WebProtocol(r),
		Hostname:        r.Host,
		EMail:           serv.contactMail,
		DurationPattern: getHtmlDurationPattern(),
	}

	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	w.WriteHeader(http.StatusOK)

	if err := t.Execute(w, data); err != nil {
		log.WithError(err).Error("Failed to execute template")
	}
}

func (serv *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	item, f, err := NewItem(r, serv.maxSize, serv.maxLifetime)
	if err == ErrLifetimeTooLong {
		log.Info("New Item with a too long lifetime was rejected")

		http.Error(w, msgLifetimeExceeds, http.StatusNotAcceptable)
		return
	} else if err == ErrFileTooBig {
		log.Info("New Item with a too great file size was rejected")

		http.Error(w, msgFileSizeExceeds, http.StatusNotAcceptable)
		return
	} else if err != nil {
		log.WithError(err).Error("Failed to create new Item")

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	} else if serv.mimeMap.MustDrop(item.ContentType) {
		log.WithField("MIME", item.ContentType).Info("Prevented upload of an illegal MIME")

		http.Error(w, msgIllegalMime, http.StatusBadRequest)
		return
	}

	itemId, err := serv.store.Put(item, f)
	if err != nil {
		log.WithError(err).Error("Failed to store Item")

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	}

	log.WithFields(log.Fields{
		"ID":      itemId,
		"expires": item.Expires,
	}).Info("Uploaded new Item")

	w.WriteHeader(http.StatusOK)

	baseUrl := fmt.Sprintf("%s://%s", WebProtocol(r), r.Host)
	onlyUrl := r.URL.Query().Has("onlyURL")

	if onlyUrl {
		fmt.Fprintf(w, "%s/%s\n", baseUrl, itemId)
	} else {
		fmt.Fprintf(w, "Fetch:   %s/%s\n", baseUrl, itemId)
		fmt.Fprintf(w, "Delete:  %s/del/%s/%s\n", baseUrl, itemId, item.DeletionKey)
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Expires: %v\n", item.Expires)
		fmt.Fprintf(w, "Burn:    %t\n", item.BurnAfterReading)
	}
}

// hasClientCachedRequest if the client submits a conditional GET, e.g., If-Modified-Since.
func (serv *Server) hasClientCachedRequest(r *http.Request, item Item) bool {
	ims, imsErr := http.ParseTime(r.Header.Get("If-Modified-Since"))
	if imsErr != nil {
		return false
	}

	return item.Created.Before(ims) && item.Expires.After(ims)
}

// handleRequestServe is called from handleRequest when a valid Item should be served.
func (serv *Server) handleRequestServe(w http.ResponseWriter, r *http.Request, item Item) error {
	f, err := serv.store.GetFile(item)
	if err != nil {
		return fmt.Errorf("reading file failed: %v", err)
	}

	defer f.Close()

	mimeType, err := serv.mimeMap.Substitute(item.ContentType)
	if err != nil {
		return fmt.Errorf("substituting MIME failed: %v", err)
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", item.Filename))

	// Original creation date might be seen as confidential.
	w.Header().Set("Last-Modified", time.Now().Format(http.TimeFormat))

	w.WriteHeader(http.StatusOK)

	// An error might happen here if the peer resets the connection, e.g., if
	// curl tries to print a non text file to stdout.
	_, _ = io.Copy(w, f)

	return nil
}

func (serv *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.WithField("method", r.Method).Debug("Request with unsupported method")

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
		log.WithError(err).WithField("ID", reqId).Warn("Requesting failed")

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	}

	if serv.hasClientCachedRequest(r, item) {
		log.WithField("ID", reqId).Debug("Requested with conditional GET; HTTP Status Code 304")
		w.WriteHeader(http.StatusNotModified)
	} else {
		err := serv.handleRequestServe(w, r, item)
		if err != nil {
			log.WithError(err).WithField("ID", reqId).Warn("Serving the request failed")

			http.Error(w, msgGenericError, http.StatusBadRequest)
			return
		}
	}

	log.WithField("ID", item.ID).Info("Item was requested")

	if item.BurnAfterReading {
		log.WithField("ID", item.ID).Info("Item will be burned")
		if err := serv.store.Delete(item); err != nil {
			log.WithError(err).WithField("ID", item.ID).Error("Deletion failed")
		}
	}
}

func (serv *Server) handleDeletion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.WithField("method", r.Method).Debug("Request with unsupported method")

		http.Error(w, msgUnsupportedMethod, http.StatusMethodNotAllowed)
		return
	}

	reqParts := strings.Split(strings.TrimLeft(r.URL.Path, "/"), "/")
	if len(reqParts) != 3 {
		log.WithField("request", reqParts).Debug("Requested URL is malformed")

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	}

	reqId, delKey := reqParts[1], reqParts[2]

	item, err := serv.store.Get(reqId, true)
	if err == ErrNotFound {
		log.WithField("ID", reqId).Debug("Requested non-existing ID")

		http.Error(w, msgNotExists, http.StatusNotFound)
		return
	} else if err != nil {
		log.WithError(err).WithField("ID", reqId).Warn("Requesting failed")

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	}

	if item.DeletionKey != delKey {
		log.WithField("ID", reqId).Warn("Deletion was requested with invalid key")

		http.Error(w, msgDeletionKeyWrong, http.StatusForbidden)
		return
	}

	if err := serv.store.Delete(item); err != nil {
		log.WithError(err).WithField("ID", reqId).Error("Requested deletion failed")

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, msgDeletionSuccess)

	log.WithField("ID", reqId).Info("Item was deleted by request")
}

// WebProtocol returns "http" or "https", based either on the X-Forwarded-Proto
// header or FastCGI's SERVER_PORT variable.
func WebProtocol(r *http.Request) string {
	fcgiParams := fcgi.ProcessEnv(r)
	if serverPort, ok := fcgiParams["SERVER_PORT"]; ok && serverPort == "443" {
		return "https"
	}

	if xfwp := r.Header.Get("X-Forwarded-Proto"); xfwp != "" {
		return xfwp
	} else {
		return "http"
	}
}
