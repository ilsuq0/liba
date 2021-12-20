package liba

import (
	"fmt"
	"log"
	"os"
)

var logger = log.New(os.Stderr, "", log.Lshortfile|log.LstdFlags)

var Verbose bool

func logf(f string, v ...interface{}) {
	if Verbose {
		logger.Output(2, fmt.Sprintf(f, v...))
	}
}
