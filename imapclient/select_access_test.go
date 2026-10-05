package imapclient_test

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectAccessMode(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		examine    bool
		want       *bool
	}{
		{name: "read write", code: "READ-WRITE", want: boolPtr(false)},
		{name: "read only", code: "READ-ONLY", want: boolPtr(true)},
		{name: "lowercase", code: "read-only", want: boolPtr(true)},
		{name: "missing"},
		{name: "examine", code: "READ-ONLY", examine: true, want: boolPtr(true)},
		{name: "examine missing", examine: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			require.NoError(t, clientConn.SetDeadline(time.Now().Add(10*time.Second)))
			require.NoError(t, serverConn.SetDeadline(time.Now().Add(10*time.Second)))
			defer serverConn.Close()
			commands := make(chan string, 1)
			serverErr := make(chan error, 1)
			go func() {
				_, err := fmt.Fprint(serverConn, "* PREAUTH [CAPABILITY IMAP4rev1] ready\r\n")
				if err != nil {
					serverErr <- err
					return
				}
				line, err := bufio.NewReader(serverConn).ReadString('\n')
				if err != nil {
					serverErr <- err
					return
				}
				commands <- line
				fields := strings.Fields(line)
				suffix := ""
				if tc.code != "" {
					suffix = "[" + tc.code + "] "
				}
				_, err = fmt.Fprintf(serverConn, "* 1 EXISTS\r\n* FLAGS (\\Seen)\r\n* OK [UIDVALIDITY 7] valid\r\n* OK [UIDNEXT 8] next\r\n%s OK %sselected\r\n", fields[0], suffix)
				serverErr <- err
			}()
			client := imapclient.New(clientConn, nil)
			defer client.Close()
			data, err := client.Select("INBOX", &imap.SelectOptions{ReadOnly: tc.examine}).Wait()
			require.NoError(t, err)
			require.NoError(t, <-serverErr)
			command := "SELECT"
			if tc.examine {
				command = "EXAMINE"
			}
			assert.Equal(t, command, strings.Fields(<-commands)[1])
			assert.Equal(t, tc.want, data.ReadOnly)
			assert.Equal(t, uint32(7), data.UIDValidity)
		})
	}
}

func boolPtr(value bool) *bool { return &value }
