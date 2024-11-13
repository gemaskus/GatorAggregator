package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gemaskus/GatorAggregator/internal/config"
	"github.com/gemaskus/GatorAggregator/internal/database"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type state struct {
	db            database.Queries
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

	db, err := sql.Open("postgres", cfg.DBURL)
	if err != nil {
		log.Fatalf("Failed to open the postgres database: %v", err)
	}

	dbQueries := database.New(db)
	currentState := state{
		db:            *dbQueries,
		currentConfig: &cfg,
	}

	cmds := commands{
		handlers: make(map[string]func(*state, command) error),
	}
	cmds.register("login", handlerLogin)
	cmds.register("register", handlerRegister)

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
		log.Fatalf("%v", err)
	}
}

func handlerLogin(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("Login requires only one argument")
	}

	user, err := s.db.GetUser(context.Background(), cmd.args[0])

	if err != nil {
		return fmt.Errorf("User does not exist in database: %v", err)
	}

	err = s.currentConfig.SetUser(user.Name)
	if err != nil {
		return err
	}

	fmt.Printf("Username has been set: %v\n", s.currentConfig.CurrentUserName)
	return nil
}

func handlerRegister(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("Register requires only one argument")
	}

	if _, err := s.db.GetUser(context.Background(), cmd.args[0]); err == nil {
		return fmt.Errorf("User already exists")
	}

	newUserParams := database.CreateUserParams{
		ID:        uuid.New(),
		CreatedAt: time.Now().Local(),
		UpdatedAt: time.Now().Local(),
		Name:      cmd.args[0],
	}

	newUser, err := s.db.CreateUser(context.Background(), newUserParams)
	if err != nil {
		return err
	}

	err = s.currentConfig.SetUser(newUser.Name)
	fmt.Printf("New User created: %v\n", newUser)

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
