// Package transport finds hubs and opens sessions with them.
//
// A hub is reached in reverse: the client listens on a TCP port, sends a
// CALL_ME frame naming that port to the hub over UDP, and the hub connects
// back. Discovery is a broadcast probe (NOTIFY_ME) that hubs answer with a
// broadcast of their identity. Conn carries frames over the resulting TCP
// connection.
package transport
