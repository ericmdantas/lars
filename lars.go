package lars

import (
	"net/http"
	"sync"
)

// HTTP Constant Terms and Variables
const (
	// CONNECT HTTP method
	CONNECT = "CONNECT"
	// DELETE HTTP method
	DELETE = "DELETE"
	// GET HTTP method
	GET = "GET"
	// HEAD HTTP method
	HEAD = "HEAD"
	// OPTIONS HTTP method
	OPTIONS = "OPTIONS"
	// PATCH HTTP method
	PATCH = "PATCH"
	// POST HTTP method
	POST = "POST"
	// PUT HTTP method
	PUT = "PUT"
	// TRACE HTTP method
	TRACE = "TRACE"

	//-------------
	// Media types
	//-------------

	ApplicationJSON                  = "application/json"
	ApplicationJSONCharsetUTF8       = ApplicationJSON + "; " + CharsetUTF8
	ApplicationJavaScript            = "application/javascript"
	ApplicationJavaScriptCharsetUTF8 = ApplicationJavaScript + "; " + CharsetUTF8
	ApplicationXML                   = "application/xml"
	ApplicationXMLCharsetUTF8        = ApplicationXML + "; " + CharsetUTF8
	ApplicationForm                  = "application/x-www-form-urlencoded"
	ApplicationProtobuf              = "application/protobuf"
	ApplicationMsgpack               = "application/msgpack"
	TextHTML                         = "text/html"
	TextHTMLCharsetUTF8              = TextHTML + "; " + CharsetUTF8
	TextPlain                        = "text/plain"
	TextPlainCharsetUTF8             = TextPlain + "; " + CharsetUTF8
	MultipartForm                    = "multipart/form-data"

	//---------
	// Charset
	//---------

	CharsetUTF8 = "charset=utf-8"

	//---------
	// Headers
	//---------

	AcceptEncoding     = "Accept-Encoding"
	Authorization      = "Authorization"
	ContentDisposition = "Content-Disposition"
	ContentEncoding    = "Content-Encoding"
	ContentLength      = "Content-Length"
	ContentType        = "Content-Type"
	Location           = "Location"
	Upgrade            = "Upgrade"
	Vary               = "Vary"
	WWWAuthenticate    = "WWW-Authenticate"
	XForwardedFor      = "X-Forwarded-For"
	XRealIP            = "X-Real-IP"

	basePath = "/"
	blank    = ""

	slashByte = '/'
	colonByte = ':'
	startByte = '*'
)

// Handler is the type used in registering handlers.
// NOTE: these handlers may get wrapped by the HandlerFunc
// type internally.
type Handler interface{}

// HandlerFunc is the internal handler type used for middleware and handlers
type HandlerFunc func(Context)

// HandlersChain is an array of HanderFunc handlers to run
type HandlersChain []HandlerFunc

// ContextFunc is the function that returns a new Context instance.
type ContextFunc func() Context

// LARS is the main routing instance
type LARS struct {
	routeGroup

	head *node

	// mostParams used to keep track of the most amount of
	// params in any URL and this will set the default capacity
	// of each context Params
	mostParams uint8

	pool sync.Pool

	newContext ContextFunc

	http404 HandlersChain // 404 Not Found
	http405 HandlersChain // 405 Method Not Allowed

	// Enables automatic redirection if the current route can't be matched but a
	// handler for the path with (without) the trailing slash exists.
	// For example if /foo/ is requested but a route only exists for /foo, the
	// client is redirected to /foo with http status code 301 for GET requests
	// and 307 for all other request methods.
	redirectTrailingSlash bool

	// If enabled, the router checks if another method is allowed for the
	// current route, if the current request can not be routed.
	// If this is the case, the request is answered with 'Method Not Allowed'
	// and HTTP status code 405.
	// If no other Method is allowed, the request is delegated to the NotFound
	// handler.
	handleMethodNotAllowed bool
}

var (
	default404Handler = func(c Context) {
		http.Error(c.Response(), http.StatusText(http.StatusNotFound), http.StatusNotFound)
	}

	methodNotAllowedHandler = func(c Context) {

		m, _ := c.Get("methods")
		methods := m.(chainMethods)

		res := c.Response()

		for k := range methods {
			res.Header().Add("Allow", k)
		}

		res.WriteHeader(http.StatusMethodNotAllowed)
	}
)

// New Creates and returns a new LARS instance
func New() *LARS {

	l := &LARS{
		routeGroup: routeGroup{
			middleware: make(HandlersChain, 0),
		},
		head: &node{
			static: nodes{},
		},
		mostParams:             0,
		http404:                []HandlerFunc{default404Handler},
		http405:                []HandlerFunc{methodNotAllowedHandler},
		redirectTrailingSlash:  true,
		handleMethodNotAllowed: false,
	}

	l.routeGroup.lars = l
	l.newContext = func() Context {
		return NewContext(l)
	}
	l.pool.New = func() interface{} {
		return l.newContext()
	}

	return l
}

// RegisterContext sets a custom Context pool initializer
// for use with your own custom context.
func (l *LARS) RegisterContext(fn ContextFunc) {
	l.newContext = fn
}

// Register404 alows for overriding of the not found handler function.
// NOTE: this is run after not finding a route even after redirecting with the trailing slash
func (l *LARS) Register404(notFound ...Handler) {

	chain := make(HandlersChain, len(notFound))

	for i, h := range notFound {
		chain[i] = wrapHandler(h)
	}

	l.http404 = chain
}

// SetRedirectTrailingSlash tells LARS whether to try
// and fix a URL by trying to find it
// lowercase -> with or without slash -> 404
func (l *LARS) SetRedirectTrailingSlash(set bool) {
	l.redirectTrailingSlash = set
}

// SetHandle405MethodNotAllowed tells LARS whether to
// handle the http 405 Method Not Allowed status code
func (l *LARS) SetHandle405MethodNotAllowed(set bool) {
	l.handleMethodNotAllowed = set
}

// Serve returns an http.Handler to be used.
func (l *LARS) Serve() http.Handler {

	// reserved for any logic that needs to happen before serving starts.
	// i.e. although this router does not use priority to determine route order
	// could add sorting of tree nodes here....

	return http.HandlerFunc(l.serveHTTP)
}

// Conforms to the http.Handler interface.
func (l *LARS) serveHTTP(w http.ResponseWriter, r *http.Request) {
	c := l.pool.Get().(Context)
	c.Reset(w, r)

	l.Find(c.UnderlyingContext(), true)

	c.Next(c)

	l.pool.Put(c)
}
