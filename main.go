package main

import (
	"ken/command"
	"ken/log"
)

func main() {
	command.Execute()

	log.Flush()
}
