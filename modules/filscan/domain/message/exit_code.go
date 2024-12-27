package message

import (
	"fmt"
	"github.com/filecoin-project/go-state-types/exitcode"
	"strconv"
)

const (
	Pending             = ExitCode(-1)
	FirstActorErrorCode = ExitCode(16)
	// ErrIllegalArgument indicates that a method parameter is invalid.
	ErrIllegalArgument = ExitCode(17)
	// ErrNotFound indicates that a requested resource does not exist.
	ErrNotFound = ExitCode(18)
	// ErrForbidden indicates that an action is disallowed.
	ErrForbidden = ExitCode(19)
	// ErrInsufficientFunds indicates that a balance of funds is insufficient.
	ErrInsufficientFunds = ExitCode(20)
	// ErrIllegalState indicates that an actor's internal state is invalid.
	ErrIllegalState = ExitCode(21)
	// ErrSerialization indicates a de/serialization failure within actor code.
	ErrSerialization = ExitCode(22)
	// ErrUnhandledMessage indicates that the actor cannot handle this message.
	ErrUnhandledMessage = ExitCode(23)
	// ErrUnspecified indicates that the actor failed with an unspecified error.
	ErrUnspecified = ExitCode(24)
	// ErrAssertionFailed indicates that the actor failed a user-level assertion
	ErrAssertionFailed = ExitCode(25)
	// ErrReadOnly indicates that the actor cannot perform the requested operation
	// in read-only mode.
	ErrReadOnly = ExitCode(26)

	// Common error codes stop here.  If you define a common error code above
	// this value it will have conflicting interpretations

	FirstActorSpecificExitCode = ExitCode(32)

	// SysErrContractReverted
	// ErrChannelStateUpdateAfterSettled = ExitCode(33)
	// ErrTooManyProveCommits = ExitCode(33)
	SysErrContractReverted = ExitCode(33)
)

var newNames = map[ExitCode]string{
	Pending:                    "Pending",
	FirstActorErrorCode:        "FirstActorErrorCode",
	ErrIllegalArgument:         "ErrIllegalArgument",
	ErrNotFound:                "ErrNotFound",
	ErrForbidden:               "ErrForbidden",
	ErrInsufficientFunds:       "ErrInsufficientFunds",
	ErrIllegalState:            "ErrIllegalState",
	ErrSerialization:           "ErrSerialization",
	ErrUnhandledMessage:        "ErrUnhandledMessage",
	ErrUnspecified:             "ErrUnspecified",
	ErrAssertionFailed:         "ErrAssertionFailed",
	ErrReadOnly:                "ErrReadOnly",
	FirstActorSpecificExitCode: "FirstActorSpecificExitCode",
	SysErrContractReverted:     "SysErrContractReverted",
}

type ExitCode int64

func (e ExitCode) String() string {
	if e == 0 {
		return "Ok"
	}

	if 0 < e && e <= 15 {
		return exitcode.ExitCode(e).String()
	}

	name, ok := newNames[e]
	if ok {
		return fmt.Sprintf("%s(%d)", name, e)
	}
	return strconv.FormatInt(int64(e), 10)
}
