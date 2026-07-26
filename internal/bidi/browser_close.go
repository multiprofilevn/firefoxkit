package bidi

import "context"

// CloseBrowser issues browser.close — asks the browser to shut itself down and
// end the session. The peer drops the WebSocket while exiting, so a transport
// error (or ErrClientClosed) here is a normal outcome, not a failure: callers
// use it as a best-effort graceful quit before killing the process.
func (c *Client) CloseBrowser(ctx context.Context) error {
	return c.Send(ctx, "browser.close", struct{}{}, nil)
}
