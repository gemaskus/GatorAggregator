package main

import (
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
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

type RSSFeed struct {
	Channel struct {
		Title       string    `xml:"title"`
		Link        string    `xml:"link"`
		Description string    `xml:"description"`
		Item        []RSSItem `xml:"item"`
	} `xml:"channel"`
}

type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
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
	cmds.register("reset", handlerReset)
	cmds.register("users", handlerUsers)
	cmds.register("agg", handlerAgg)
	cmds.register("addfeed", handlerAddFeed)
	cmds.register("feeds", handlerFeeds)

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

func handlerReset(s *state, cmd command) error {
	if len(cmd.args) != 0 {
		return fmt.Errorf("too many arguments for the reset command.")
	}

	err := s.db.ResetFeeds(context.Background())
	if err != nil {
		return err
	}
	err = s.db.ResetUsers(context.Background())
	if err != nil {
		return err
	}

	return nil
}

func handlerUsers(s *state, cmd command) error {
	if len(cmd.args) != 0 {
		return fmt.Errorf("too many arguments for the get users list command")
	}

	users, err := s.db.GetUsers(context.Background())
	if err != nil {
		return err
	}

	for _, user := range users {
		if user.Name == s.currentConfig.CurrentUserName {
			fmt.Printf("* %s (current)\n", user.Name)
		} else {
			fmt.Printf("* %s\n", user.Name)
		}
	}
	return nil
}

func handlerAgg(s *state, cmd command) error {

	url := "https://www.wagslane.dev/index.xml"

	RSSfeed, err := fetchFeed(context.Background(), url)
	if err != nil {
		return err
	}

	fmt.Printf("%v", RSSfeed)
	return nil
}

func handlerAddFeed(s *state, cmd command) error {
	if len(cmd.args) < 2 {
		return fmt.Errorf("Too few arguments for adding a feed")
	}

	if len(cmd.args) > 2 {
		return fmt.Errorf("Too many arguments for adding a feed")
	}
	currentUser, err := s.db.GetUser(context.Background(), s.currentConfig.CurrentUserName)
	if err != nil {
		return fmt.Errorf("This really shouldn't happen, unless you try to add a fed without having any users before hand")
	}

	newFeedParams := database.CreateFeedParams{
		ID:        uuid.New(),
		CreatedAt: time.Now().Local(),
		UpdatedAt: time.Now().Local(),
		Name:      cmd.args[0],
		Url:       cmd.args[1],
		UserID:    currentUser.ID,
	}

	newFeed, err := s.db.CreateFeed(context.Background(), newFeedParams)
	if err != nil {
		return err
	}

	fmt.Printf("New Feed Created: %v\n", newFeed)

	return nil
}

func handlerFeeds(s *state, cmd command) error {
	if len(cmd.args) > 0 {
		return fmt.Errorf("Too many arguments for listing active feeds")
	}

	feeds, err := s.db.GetFeeds(context.Background())
	if err != nil {
		return fmt.Errorf("Could not retreive feeds: %v", err)
	}

	for _, feed := range feeds {
		fmt.Printf("Feed name: %s\n", feed.Name)
		fmt.Printf("Feed URL: %s\n", feed.Url)
		user, err := s.db.GetUserByID(context.Background(), feed.UserID)
		if err != nil {
			return fmt.Errorf("Could not retrieve user by ID: %v", err)
		}
		fmt.Printf("Feed created by: %s\n", user.Name)
	}

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

func fetchFeed(ctx context.Context, feedURL string) (*RSSFeed, error) {
	request, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		return &RSSFeed{}, err
	}
	request.Header.Add("User-Agent", "gator")

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return &RSSFeed{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &RSSFeed{}, err
	}
	newRSSFeed := RSSFeed{}
	err = xml.Unmarshal(body, &newRSSFeed)
	if err != nil {
		return &RSSFeed{}, nil
	}

	newRSSFeed.Channel.Title = html.UnescapeString(newRSSFeed.Channel.Title)
	newRSSFeed.Channel.Description = html.UnescapeString(newRSSFeed.Channel.Description)
	for _, item := range newRSSFeed.Channel.Item {
		item.Title = html.UnescapeString(item.Title)
		item.Description = html.UnescapeString(item.Description)
	}

	return &newRSSFeed, nil

}
