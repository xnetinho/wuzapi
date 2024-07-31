package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	_ "modernc.org/sqlite"
)

type server struct {
	db     *sql.DB
	router *mux.Router
	exPath string
}

var (
	address    = flag.String("address", "", "Bind IP Address")
	port       = flag.String("port", "8080", "Listen Port")
	waDebug    = flag.String("wadebug", "", "Enable whatsmeow debug (INFO or DEBUG)")
	logType    = flag.String("logtype", "console", "Type of log output (console or json)")
	sslcert    = flag.String("sslcertificate", "", "SSL Certificate File")
	sslprivkey = flag.String("sslprivatekey", "", "SSL Certificate Private Key File")
	adminToken = flag.String("admintoken", "", "Security Token to authorize admin actions (list/create/remove users)")

	dbType        string
	container     *sqlstore.Container
	killchannel   = make(map[int](chan bool))
	userinfocache = cache.New(5*time.Minute, 10*time.Minute)
)

func init() {
	flag.Parse()

	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	log.Logger = zerolog.New(output).With().Timestamp().Logger()

	if os.Getenv("ADDRESS") != "" {
		*address = os.Getenv("ADDRESS")
	}
	if os.Getenv("PORT") != "" {
		*port = os.Getenv("PORT")
	}
	if os.Getenv("WADEBUG") != "" {
		*waDebug = os.Getenv("WADEBUG")
	}
	if os.Getenv("LOGTYPE") != "" {
		*logType = os.Getenv("LOGTYPE")
	}
	if os.Getenv("SSLCERT") != "" {
		*sslcert = os.Getenv("SSLCERT")
	}
	if os.Getenv("SSLPRIVKEY") != "" {
		*sslprivkey = os.Getenv("SSLPRIVKEY")
	}
	if os.Getenv("ADMINTOKEN") != "" {
		*adminToken = os.Getenv("ADMINTOKEN")
	}
}

func main() {
	ex, err := os.Executable()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get executable path")
	}
	exPath := filepath.Dir(ex)

	dbType = os.Getenv("DB_TYPE")
	if dbType == "" {
		log.Fatal().Msg("DB_TYPE environment variable is not set")
	}

	var appDB *sql.DB
	var waDB *sqlstore.Container

	switch dbType {
	case "sqlite3":
		appDBPath := os.Getenv("APP_DB_PATH")
		waDBPath := os.Getenv("WA_DB_PATH")

		if appDBPath == "" || waDBPath == "" {
			log.Fatal().Msg("APP_DB_PATH or WA_DB_PATH environment variable is not set")
		}

		appDB, err = sql.Open("sqlite", "file:"+appDBPath+"?_foreign_keys=on")
		if err != nil {
			log.Fatal().Err(err).Msg("Could not open SQLite application database")
		}

		if *waDebug != "" {
			dbLog := waLog.Stdout("Database", *waDebug, true)
			waDB, err = sqlstore.New("sqlite3", "file:"+waDBPath+"?_foreign_keys=on&_busy_timeout=3000", dbLog)
		} else {
			waDB, err = sqlstore.New("sqlite3", "file:"+waDBPath+"?_foreign_keys=on&_busy_timeout=3000", nil)
		}
		if err != nil {
			log.Fatal().Err(err).Msg("Could not open SQLite WhatsApp database")
		}

	case "postgresql":
		host := os.Getenv("POSTGRES_HOST")
		user := os.Getenv("POSTGRES_USER")
		password := os.Getenv("POSTGRES_PASSWORD")
		appDatabase := os.Getenv("POSTGRES_APP_DATABASE")
		waDatabase := os.Getenv("POSTGRES_WA_DATABASE")

		if host == "" || user == "" || password == "" || appDatabase == "" || waDatabase == "" {
			log.Fatal().Msg("PostgreSQL environment variables are not set properly")
		}

		appConnectionString := fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable",
			host, user, password, appDatabase)
		waConnectionString := fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable",
			host, user, password, waDatabase)

		appDB, err = sql.Open("postgres", appConnectionString)
		if err != nil {
			log.Fatal().Err(err).Msg("Could not open PostgreSQL application database")
		}

		if *waDebug != "" {
			dbLog := waLog.Stdout("Database", *waDebug, true)
			waDB, err = sqlstore.New("postgres", waConnectionString, dbLog)
		} else {
			waDB, err = sqlstore.New("postgres", waConnectionString, nil)
		}
		if err != nil {
			log.Fatal().Err(err).Msg("Could not open PostgreSQL WhatsApp database")
		}

	default:
		log.Fatal().Msg("Invalid database type specified")
	}
	//	defer appDB.Close()

	s := &server{
		router: mux.NewRouter(),
		db:     appDB,
		exPath: exPath,
	}
	s.routes()

	container = waDB

	s.connectOnStartup()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	var srv *http.Server

	addr := ":" + *port
	if *address != "" {
		addr = *address + ":" + *port
	}

	srv = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	go func() {
		if *sslcert != "" && *sslprivkey != "" {
			if err := srv.ListenAndServeTLS(*sslcert, *sslprivkey); err != nil && err != http.ErrServerClosed {
				log.Fatal().Err(err).Msg("Server startup failed")
			}
		} else {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatal().Err(err).Msg("Server startup failed")
			}
		}
	}()

	log.Info().Str("address", *address).Str("port", *port).Msg("Server started")

	<-done
	log.Info().Msg("Server stopped")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server shutdown failed")
	}
	log.Info().Msg("Server exited properly")
}