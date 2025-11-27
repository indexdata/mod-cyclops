package main

import "os"
import "fmt"

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "Usage:", os.Args[0])
		os.Exit(1)
	}

	/*
	server, err := MakeCyclopsServer(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: cannot create server: %s\n", os.Args[0], err)
		os.Exit(2)
	}

	err = server.launch()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: cannot launch server: %s\n", os.Args[0], err)
		os.Exit(3)
	}
	*/

	fmt.Printf("Hello, world!\n")
}
