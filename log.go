package socket

import (
	l "log"
	"os"
	"runtime"
)

var log = l.New(os.Stderr, "", l.Lmicroseconds|l.Llongfile|l.Ldate)

func DumpStack() {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	log.Println(string(buf))

}
