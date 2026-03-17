package services

import "errors"

var (
	ErrAccountNotInitialized = errors.New("account not initialized")
	ErrOldPasswordRequired   = errors.New("old password required")
	ErrOldPasswordInvalid    = errors.New("old password invalid")
	ErrPasswordUnchanged     = errors.New("new password unchanged")
)
