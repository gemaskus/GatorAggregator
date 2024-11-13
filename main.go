package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gemaskus/GatorAggregator/internal/config"
)

type state struct {
	currentConfig *config.Config
}

type command struct {
	name string
	args []string
}

type commands struct {
	handlers map[string]func(*state, command) error
}

func main() {
	cfg, err := config.Read()
	if err != nil {
		log.Fatalf("error reading the config file: %v", err)
	}
	currentState := state{
		currentConfig: &cfg,
	}

	cmds := commands{
		handlers: make(map[string]func(*state, command) error),
	}
	cmds.register("login", handlerLogin)

	args := os.Args

	if len(args) < 2 {
		log.Fatalf("Too few arguments: %d", len(args))
	}
	cmd := command{
		name: args[1],
		args: args[2:],
	}

	err = cmds.run(&currentState, cmd)
	if err != nil {
		log.Fatal("A username is required")
	}
}

func handlerLogin(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("Login requires only one argument")
	}
	err := s.currentConfig.SetUser(cmd.args[0])
	if err != nil {
		return err
	}

	fmt.Printf("Username has been set: %v\n", s.currentConfig.CurrentUserName)
	return nil
}

func (cmds *commands) register(name string, f func(*state, command) error) {
	cmds.handlers[name] = f
}

func (cmds *commands) run(s *state, cmd command) error {
	if handler, exists := cmds.handlers[cmd.name]; exists {
		return handler(s, cmd)
	}
	return fmt.Errorf("Command not found: %s", cmd.name)
}
