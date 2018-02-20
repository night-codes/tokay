package tokay

import (
	"io/ioutil"
	lg "log"
	"os"
)

var (
	Trace   = lg.New(ioutil.Discard, "[TRACE] ", lg.Ldate|lg.Ltime|lg.Lshortfile)
	Debug   = lg.New(os.Stdout, "[Tokay] ", 0)
	Info    = lg.New(os.Stdout, "[INFO] ", lg.Ldate|lg.Ltime|lg.Lshortfile)
	Warning = lg.New(os.Stdout, "[WARNING] ", lg.Ldate|lg.Ltime|lg.Lshortfile)
	Error   = lg.New(os.Stderr, "[ERROR] ", lg.Ldate|lg.Ltime|lg.Lshortfile)
	log     = Info
)
