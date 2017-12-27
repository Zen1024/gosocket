package socket

import (
	l "log"
	"os"
)

var log = l.New(os.Stderr, "", l.Lmicroseconds|l.Llongfile|l.Ldate)
