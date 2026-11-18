package imapclient

import (
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// Move sends a MOVE command.
//
// If the server doesn't support IMAP4rev2 nor the MOVE extension, a fallback
// with COPY + STORE + EXPUNGE commands is used.
func (c *Client) Move(numSet imap.NumSet, mailbox string) *MoveCommand {
	return c.MoveWithReceipt(numSet, mailbox, nil)
}

// MoveWithReceipt sends MOVE and invokes receipt synchronously when COPYUID is
// parsed, before reading further responses. The callback must not call this client.
// A callback error is returned by Wait even if the command later succeeds.
func (c *Client) MoveWithReceipt(numSet imap.NumSet, mailbox string, receipt func(MoveData) error) *MoveCommand {
	// If the server doesn't support MOVE, fallback to [UID] COPY,
	// [UID] STORE +FLAGS.SILENT \Deleted and [UID] EXPUNGE
	cmdName := "MOVE"
	if !c.Caps().Has(imap.CapMove) {
		cmdName = "COPY"
	}

	cmd := &MoveCommand{receipt: receipt}
	enc := c.beginCommand(uidCmdName(cmdName, imapwire.NumSetKind(numSet)), cmd)
	enc.SP().NumSet(numSet).SP().Mailbox(mailbox)
	enc.end()

	if cmdName == "COPY" {
		cmd.store = c.Store(numSet, &imap.StoreFlags{
			Op:     imap.StoreFlagsAdd,
			Silent: true,
			Flags:  []imap.Flag{imap.FlagDeleted},
		}, nil)
		if uidSet, ok := numSet.(imap.UIDSet); ok && c.Caps().Has(imap.CapUIDPlus) {
			cmd.expunge = c.UIDExpunge(uidSet)
		} else {
			cmd.expunge = c.Expunge()
		}
	}

	return cmd
}

// MoveCommand is a MOVE command.
type MoveCommand struct {
	commandBase
	data       MoveData
	receipt    func(MoveData) error
	receiptErr error

	// Fallback
	store   *FetchCommand
	expunge *ExpungeCommand
}

func (cmd *MoveCommand) Wait() (*MoveData, error) {
	err := cmd.wait()
	if cmd.receiptErr != nil {
		if err != nil {
			return nil, fmt.Errorf("MOVE completion: %v; receipt: %w", err, cmd.receiptErr)
		}
		return nil, cmd.receiptErr
	}
	if err != nil {
		return nil, err
	}
	if cmd.store != nil {
		if err := cmd.store.Close(); err != nil {
			return nil, err
		}
	}
	if cmd.expunge != nil {
		if err := cmd.expunge.Close(); err != nil {
			return nil, err
		}
	}
	return &cmd.data, nil
}

func (cmd *MoveCommand) recordReceipt(uidValidity uint32, srcUIDs, dstUIDs imap.NumSet) {
	cmd.data = MoveData{UIDValidity: uidValidity, SourceUIDs: srcUIDs, DestUIDs: dstUIDs}
	if cmd.receipt != nil {
		if err := cmd.receipt(cmd.data); err != nil && cmd.receiptErr == nil {
			cmd.receiptErr = err
		}
	}
}

// MoveData contains the data returned by a MOVE command.
type MoveData struct {
	// requires UIDPLUS or IMAP4rev2
	UIDValidity uint32
	SourceUIDs  imap.NumSet
	DestUIDs    imap.NumSet
}
