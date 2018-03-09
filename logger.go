package tokay

import (
	"io/ioutil"
	lg "log"
	"os"
)

var (
	trace    = lg.New(ioutil.Discard, "[TRACE] ", lg.Ldate|lg.Ltime|lg.Lshortfile)
	debug    = lg.New(os.Stdout, "[Tokay] ", 0)
	info     = lg.New(os.Stdout, "[INFO] ", lg.Ldate|lg.Ltime|lg.Lshortfile)
	warning  = lg.New(os.Stdout, "[WARNING] ", lg.Ldate|lg.Ltime|lg.Lshortfile)
	errorlog = lg.New(os.Stderr, "[ERROR] ", lg.Ldate|lg.Ltime|lg.Lshortfile)
	log      = info
)
