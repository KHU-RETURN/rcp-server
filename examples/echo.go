package main

import (
	"io"
	"os"
)

func main() {
	input, _ := io.ReadAll(os.Stdin)
	_, _ = os.Stdout.Write(input)
}
