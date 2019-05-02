package vsphere

import (
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// IsInvalidCredentialsError returns true if error is of type InvalidLogin
func IsInvalidCredentialsError(err error) bool {
	isInvalidCredentialsError := false
	if soap.IsSoapFault(err) {
		_, isInvalidCredentialsError = soap.ToSoapFault(err).VimFault().(types.InvalidLogin)
	}
	return isInvalidCredentialsError
}
