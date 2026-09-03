package app

import (
	"context"
	"fmt"
)

func Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Println("devdash")
		return nil
	}

	switch args[0] {
	case "doctor":
		return runDoctor(ctx)

	case "help", "-h", "--help":
		printHelp()
		return nil

	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func printHelp() {
	fmt.Println(`Usage:
  devdash
  devdash doctor
  devdash help`)
}
