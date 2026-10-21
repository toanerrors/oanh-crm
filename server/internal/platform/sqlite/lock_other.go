//go:build !unix

package sqlite

import (
	"fmt"
	"os"
)

func lockRoot(string) (*os.File, error) {
	return nil, fmt.Errorf("exclusive dataset ownership has not been verified on this operating system")
}
