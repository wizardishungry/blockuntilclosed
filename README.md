When working on a [fasthttp](https://github.com/valyala/fasthttp) project, I discovered
[there isn't a mechanism](https://github.com/valyala/fasthttp/issues/965) to receive notification (via
`context.Context`) of request aborts.

I noticed that `fasthttp.RequestCtx` provides a mechanism for getting at the underlying `net.Conn` for an
http request (`reqCtx.Conn()`). Since fasthttp does not support http/2, and thus connection multiplexing,
a socket disconnect generally corresponds to an aborted request.

This is a PoC that uses kernel async mechanisms (`kqueue` on Mac, `epoll` on Linux) to subscribe to 
notifications of socket disconnects. It is possible to derive a `context.Context` that aborts upon
disconnect notification.

**Do not use this!**
 - It has not been battle tested.
 - A connection may still have readable data on it after a disconnection signal has been received.
 More work is necessary.

This may be generalized to `os.File` on some platforms (see notes in code), but for now it is only known
to "work" on Mac and Linux for TCP & Unix sockets.

## Reading list

- https://github.com/golang/go/issues/15735
- https://github.com/tidwall/evio/blob/master/internal/internal_linux.go epoll example