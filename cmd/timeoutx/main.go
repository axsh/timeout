package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println("timeoutx 0.3.0-dev")
		return
	}
	fmt.Fprintln(os.Stderr, "timeoutx: use 'timeoutx run' (full CLI lands in next implementation steps)")
	fmt.Fprintln(os.Stderr, "usage: timeoutx <run|shell-init|version> ...")
	os.Exit(0)
}
