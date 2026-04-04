package access

import (
	"context"
	"fmt"
	"io"
	"log"

	gossh "golang.org/x/crypto/ssh"
)

// handleDirectRelay implements Mode 2: ProxyJump-style TCP relay.
//
// The SSH client (user) expects a "direct-tcpip" forwarding channel.
// RCP accepts the channel, dials the target VM through the namespace,
// and bridges raw bytes between the two sides. The user's SSH client
// then completes the SSH handshake directly with the VM.
func (h *ConnectionHandler) handleDirectRelay(
	_ *gossh.ServerConn,
	chans <-chan gossh.NewChannel,
	email, vmName string,
) {
	ctx := context.Background()

	vm, err := h.svc.ResolveVM(ctx, email, vmName)
	if err != nil {
		log.Printf("direct relay: resolve vm %q for %s: %v", vmName, email, err)
		drainSSHChannels(chans, fmt.Sprintf("vm %q not found", vmName))
		return
	}

	for newChan := range chans {
		if newChan.ChannelType() != "direct-tcpip" {
			_ = newChan.Reject(gossh.UnknownChannelType, "only direct-tcpip supported in direct mode")
			continue
		}

		ch, reqs, err := newChan.Accept()
		if err != nil {
			log.Printf("direct relay: accept channel: %v", err)
			return
		}
		go gossh.DiscardRequests(reqs)

		// Dial VM through namespace for each channel (SSH multiplexing support)
		nsConn, err := h.svc.DialVM(ctx, vm.FixedIP)
		if err != nil {
			log.Printf("direct relay: dial vm %s: %v", vm.FixedIP, err)
			_ = ch.Close()
			return
		}

		go bridgeSSHStreams(ch, nsConn)
	}
}

// bridgeSSHStreams copies bytes bidirectionally between a and b until either side closes.
func bridgeSSHStreams(a io.ReadWriteCloser, b io.ReadWriteCloser) {
	done := make(chan struct{}, 2)

	go func() {
		_, _ = io.Copy(a, b)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(b, a)
		done <- struct{}{}
	}()

	<-done
	_ = a.Close()
	_ = b.Close()
}

// drainSSHChannels rejects all pending channel requests with the given reason.
func drainSSHChannels(chans <-chan gossh.NewChannel, reason string) {
	for newChan := range chans {
		_ = newChan.Reject(gossh.ConnectionFailed, reason)
	}
}
