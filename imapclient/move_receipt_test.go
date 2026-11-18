package imapclient_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoveReceiptBeforeCompletion(t *testing.T) {
	for _, tc := range []struct {
		name, completion string
		receiptErr       bool
	}{
		{"tagged-no", "NO refused", false}, {"disconnect", "", false}, {"tagged-ok", "OK completed", false}, {"save-failed", "OK completed", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			client := imapclient.New(clientConn, nil)
			t.Cleanup(func() { _ = client.Close(); _ = serverConn.Close() })
			entered := make(chan imapclient.MoveData, 1)
			release := make(chan struct{})
			serverDone := make(chan struct{})
			go func() {
				defer close(serverDone)
				reader := bufio.NewReader(serverConn)
				_, _ = io.WriteString(serverConn, "* OK [CAPABILITY IMAP4rev1 MOVE UIDPLUS] ready\r\n")
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				tag, _, _ := strings.Cut(line, " ")
				_, _ = io.WriteString(serverConn, "* OK [COPYUID 42 7 9] moved\r\n")
				<-release
				if tc.completion == "" {
					_ = serverConn.Close()
					return
				}
				_, _ = fmt.Fprintf(serverConn, "%s %s\r\n", tag, tc.completion)
			}()
			saveErr := errors.New("save failed")
			cmd := client.MoveWithReceipt(imap.UIDSetNum(7), "Archive", func(data imapclient.MoveData) error {
				entered <- data
				if tc.receiptErr {
					return saveErr
				}
				return nil
			})
			receipt := <-entered
			assert.Equal(t, uint32(42), receipt.UIDValidity)
			assert.Equal(t, "7", receipt.SourceUIDs.String())
			assert.Equal(t, "9", receipt.DestUIDs.String())
			close(release)
			_, err := cmd.Wait()
			if tc.receiptErr {
				require.ErrorIs(t, err, saveErr)
			} else if tc.completion != "OK completed" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			<-serverDone
		})
	}
}

func TestMoveTaggedCopyUIDReceipt(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := imapclient.New(clientConn, nil)
	t.Cleanup(func() { _ = client.Close(); _ = serverConn.Close() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		reader := bufio.NewReader(serverConn)
		_, _ = io.WriteString(serverConn, "* OK [CAPABILITY IMAP4rev1 MOVE UIDPLUS] ready\r\n")
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		tag, _, _ := strings.Cut(line, " ")
		_, _ = fmt.Fprintf(serverConn, "%s NO [COPYUID 42 7 9] rejected\r\n", tag)
	}()
	receipts := make(chan imapclient.MoveData, 1)
	_, err := client.MoveWithReceipt(imap.UIDSetNum(7), "Archive", func(data imapclient.MoveData) error { receipts <- data; return nil }).Wait()
	require.Error(t, err)
	receipt := <-receipts
	assert.Equal(t, uint32(42), receipt.UIDValidity)
	assert.Equal(t, "9", receipt.DestUIDs.String())
	<-done
}
