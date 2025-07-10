//go:build !bundle

package bundle

import (
	"net/http"

	"github.com/gozelle/fs"
	logging "github.com/gozelle/logger"
	"github.com/gozelle/vfs"
)

var Templates http.FileSystem
var log = logging.NewLogger("fevm-bundle")

func init() {
	path, err := fs.Lookup("modules/fevm/protocol")
	if err != nil {
		log.Warnf("register fevm bundle error: %s", err)
	}
	log.Debugf("bundle proxy path: %s", path)
	Templates = vfs.Proxy(path)
}
