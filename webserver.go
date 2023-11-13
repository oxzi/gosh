package main

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"strings"
	"time"

	_ "embed"
)

//go:embed index.html
var defaultIndexTpl string

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
	store       *StoreRpcClient
	maxSize     int64
	maxLifetime time.Duration
	contactMail string
	mimeDrop    map[string]struct{}
	mimeMap     map[string]string
	urlPrefix   string
	indexTpl    *template.Template
	staticFiles map[string]StaticFileConfig
}

// NewServer creates a new Server with a given database directory, and
// configuration values. The Server must be started as an http.Handler.
func NewServer(
	store *StoreRpcClient,
	maxSize int64,
	maxLifetime time.Duration,
	contactMail string,
	mimeDrop map[string]struct{},
	mimeMap map[string]string,
	urlPrefix string,
	indexTplRaw string,
	staticFiles map[string]StaticFileConfig,
) (s *Server, err error) {
	indexTpl := defaultIndexTpl
	if indexTplRaw != "" {
		indexTpl = indexTplRaw
	}

	t, err := template.New("index").Parse(indexTpl)
	if err != nil {
		return nil, err
	}

	s = &Server{
		store:       store,
		maxSize:     maxSize,
		maxLifetime: maxLifetime,
		contactMail: contactMail,
		mimeDrop:    mimeDrop,
		mimeMap:     mimeMap,
		urlPrefix:   urlPrefix,
		indexTpl:    t,
		staticFiles: staticFiles,
	}
	return
}

// ServeFcgi starts an FastCGI listener on the given file descriptor.
func (serv *Server) ServeFcgi(fd *os.File) error {
	ln, err := net.FileListener(fd)
	if err != nil {
		return err
	}

	return fcgi.Serve(ln, serv)
}

// ServeHttpd starts an HTTPD listener on the given file descriptor.
func (serv *Server) ServeHttpd(fd *os.File) error {
	webServer := &http.Server{Handler: serv}
	ln, err := net.FileListener(fd)
	if err != nil {
		return err
	}

	return webServer.Serve(ln)
}

// Close the Server and its components.
func (serv *Server) Close() error {
	return serv.store.Close()
}

func (serv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, reqPath, _ := strings.Cut(r.URL.Path, serv.urlPrefix)
	if reqPath == "" {
		http.RedirectHandler(serv.urlPrefix+"/", http.StatusTemporaryRedirect).ServeHTTP(w, r)
	} else if reqPath == "/" {
		serv.handleRoot(w, r)
	} else if strings.HasPrefix(reqPath, "/del/") {
		serv.handleDeletion(w, r)
	} else if stc, ok := serv.staticFiles[reqPath]; ok {
		serv.handleStaticFile(w, r, stc)
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
		slog.Debug("Called with unsupported method", slog.String("method", r.Method))

		http.Error(w, msgUnsupportedMethod, http.StatusMethodNotAllowed)
	}
}

func (serv *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Expires         string
		Size            string
		Proto           string
		Hostname        string
		Prefix          string
		EMail           string
		DurationPattern string
	}{
		Expires:         PrettyDuration(serv.maxLifetime),
		Size:            PrettyBytesize(serv.maxSize),
		Proto:           WebProtocol(r),
		Hostname:        r.Host,
		Prefix:          serv.urlPrefix,
		EMail:           serv.contactMail,
		DurationPattern: getHtmlDurationPattern(),
	}

	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	w.WriteHeader(http.StatusOK)

	if err := serv.indexTpl.Execute(w, data); err != nil {
		slog.Error("Failed to execute template", slog.Any("error", err))
	}
}

func (serv *Server) handleStaticFile(w http.ResponseWriter, r *http.Request, sfc StaticFileConfig) {
	if r.Method != http.MethodGet {
		slog.Debug("Request with unsupported method", slog.String("method", r.Method))

		http.Error(w, msgUnsupportedMethod, http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", sfc.Mime)
	w.WriteHeader(http.StatusOK)

	staticReader := bytes.NewReader(sfc.data)
	_, err := io.Copy(w, staticReader)
	if err != nil {
		slog.Error("Failed to write static file back to request", slog.Any("error", err))

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	}
}

func (serv *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	item, f, err := NewItemFromRequest(r, serv.maxSize, serv.maxLifetime)
	if err == ErrLifetimeTooLong {
		slog.Info("New Item with a too long lifetime was rejected")

		http.Error(w, msgLifetimeExceeds, http.StatusNotAcceptable)
		return
	} else if err == ErrFileTooBig {
		slog.Info("New Item with a too great file size was rejected")

		http.Error(w, msgFileSizeExceeds, http.StatusNotAcceptable)
		return
	} else if err != nil {
		slog.Error("Failed to create new Item", slog.Any("error", err))

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	} else if _, drop := serv.mimeDrop[item.ContentType]; drop {
		slog.Info("Prevented upload of an illegal MIME", slog.String("mime", item.ContentType))

		http.Error(w, msgIllegalMime, http.StatusBadRequest)
		return
	}

	itemId, err := serv.store.Put(item, f, context.Background())
	if err != nil {
		slog.Error("Failed to store Item", slog.Any("error", err))

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	}

	slog.Info("Uploaded new Item",
		slog.String("id", itemId), slog.Any("expires", item.Expires))

	w.WriteHeader(http.StatusOK)

	baseUrl := fmt.Sprintf("%s://%s%s", WebProtocol(r), r.Host, serv.urlPrefix)
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
	f, err := serv.store.GetFile(item.ID, context.Background())
	if err != nil {
		return fmt.Errorf("reading file failed: %v", err)
	}

	defer f.Close()

	mimeType := item.ContentType
	if mimeSubst, ok := serv.mimeMap[mimeType]; ok {
		mimeType = mimeSubst
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
		slog.Debug("Request with unsupported method", slog.String("method", r.Method))

		http.Error(w, msgUnsupportedMethod, http.StatusMethodNotAllowed)
		return
	}

	_, reqId, _ := strings.Cut(r.URL.Path, serv.urlPrefix)
	reqId = strings.TrimLeft(reqId, "/")

	item, err := serv.store.Get(reqId, context.Background())
	if err == ErrNotFound {
		slog.Debug("Requested non-existing ID", slog.String("id", reqId))

		http.Error(w, msgNotExists, http.StatusNotFound)
		return
	} else if err != nil {
		slog.Warn("Failed to request", slog.String("id", reqId), slog.Any("error", err))

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	}

	if serv.hasClientCachedRequest(r, item) {
		slog.Debug("Requested with conditional GET; HTTP Status Code 304", slog.String("id", reqId))
		w.WriteHeader(http.StatusNotModified)
	} else {
		err := serv.handleRequestServe(w, r, item)
		if err != nil {
			slog.Warn("Failed to serve request",
				slog.Any("error", err), slog.String("id", reqId))

			http.Error(w, msgGenericError, http.StatusBadRequest)
			return
		}
	}

	slog.Info("Item was requested", slog.String("id", item.ID))

	if item.BurnAfterReading {
		slog.Info("Item will be burned", slog.String("id", item.ID))
		if err := serv.store.Delete(item.ID, context.Background()); err != nil {
			slog.Error("Failed to delete Item",
				slog.String("id", item.ID), slog.Any("error", err))
		}
	}
}

func (serv *Server) handleDeletion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		slog.Debug("Request with unsupported method", slog.String("method", r.Method))

		http.Error(w, msgUnsupportedMethod, http.StatusMethodNotAllowed)
		return
	}

	_, reqId, _ := strings.Cut(r.URL.Path, serv.urlPrefix)
	reqId = strings.TrimLeft(reqId, "/")
	reqParts := strings.Split(reqId, "/")

	if len(reqParts) != 3 {
		slog.Debug("Requested URL is malformed", slog.Any("request", reqParts))

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	}

	reqId, delKey := reqParts[1], reqParts[2]

	item, err := serv.store.Get(reqId, context.Background())
	if err == ErrNotFound {
		slog.Debug("Requested non-existing ID", slog.String("id", reqId))

		http.Error(w, msgNotExists, http.StatusNotFound)
		return
	} else if err != nil {
		slog.Warn("Failed to request", slog.String("id", reqId), slog.Any("error", err))

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	}

	if item.DeletionKey != delKey {
		slog.Warn("Deletion was requested with invalid key", slog.String("id", reqId))

		http.Error(w, msgDeletionKeyWrong, http.StatusForbidden)
		return
	}

	if err := serv.store.Delete(item.ID, context.Background()); err != nil {
		slog.Error("Failed to delete", slog.String("id", reqId), slog.Any("error", err))

		http.Error(w, msgGenericError, http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, msgDeletionSuccess)

	slog.Info("Item was deleted by request", slog.String("id", reqId))
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
