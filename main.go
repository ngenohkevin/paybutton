package main

import (
	"github.com/ngenohkevin/paybutton/internals/server"
	"log/slog"
	"os"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	srv, err := server.NewServer(logger)
	if err != nil {
		logger.Error("Error creating server:", slog.String("error", err.Error()))
		os.Exit(1)
	}
	//start the server
	if err := srv.Start(); err != nil {
		logger.Error("Error starting server:", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
