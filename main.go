package main

import (
	"context"
	"encoding/json"
	"github.com/GeertJohan/go.rice"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/minio/minio-go"
	"github.com/nkonev/blog-storage/client"
	"github.com/nkonev/blog-storage/data/mongo_lock"
	"github.com/nkonev/blog-storage/data/repository"
	"github.com/nkonev/blog-storage/handlers"
	. "github.com/nkonev/blog-storage/logger"
	"github.com/nkonev/blog-storage/utils"
	"github.com/spf13/viper"
	migrate "github.com/xakep666/mongo-migrate"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/fx"
	"net/http"
	"regexp"
	"strings"
)

const SESSION_COOKIE = "SESSION"
const AUTH_URL = "auth.url"
const LOCK_COLLECTION = "migration_lock"

type authMiddleware echo.MiddlewareFunc
type staticMiddleware echo.MiddlewareFunc

func main() {
	configFile, clearMongo, clearMinio := utils.InitFlag("./config-dev/config.yml")
	utils.InitViper(configFile)

	app := fx.New(
		fx.Logger(Logger),
		fx.Provide(
			configureMongo,
			configureMinio,
			repository.NewUserFileRepository,
			repository.NewLimitsRepository,
			handlers.NewFsHandler,
			configureEcho,
			configureMigrate,
			configureAuthMiddleware,
			configureStaticMiddleware,
			client.NewRestClient,
		),
		fx.Invoke(runMigrate, processCli(clearMongo, clearMinio), runEcho),
	)
	app.Run()

	Logger.Infof("Exit program")
}

func configureEcho(fsh *handlers.FsHandler, authMiddleware authMiddleware, staticMiddleware staticMiddleware, lc fx.Lifecycle) *echo.Echo {
	bodyLimit := viper.GetString("server.body.limit")

	e := echo.New()
	e.Logger.SetOutput(Logger.Writer())

	e.Pre(echo.MiddlewareFunc(staticMiddleware))
	e.Use(echo.MiddlewareFunc(authMiddleware))

	accessLoggerConfig := middleware.LoggerConfig{
		Output: Logger.Writer(),
		Format: `"remote_ip":"${remote_ip}",` +
			`"method":"${method}","uri":"${uri}",` +
			`"status":${status},"error":"${error}","latency_human":"${latency_human}"` +
			`,"bytes_in":${bytes_in},"bytes_out":${bytes_out},"user_agent":"${user_agent}"` + "\n",
	}
	e.Use(middleware.LoggerWithConfig(accessLoggerConfig))
	e.Use(middleware.Secure())
	e.Use(middleware.BodyLimit(bodyLimit))

	e.GET("/ls", fsh.LsHandler)
	e.GET("/limits", fsh.Limits)
	e.POST("/upload", fsh.UploadHandler)
	e.GET(utils.DOWNLOAD_PREFIX+":file", fsh.DownloadHandler)
	e.POST("/rename/:file", fsh.MoveHandler)
	e.DELETE("/delete/:file", fsh.DeleteHandler)
	e.PUT("/publish/:file", fsh.Publish)
	e.GET(utils.PUBLIC_PREFIX+"/"+utils.USER_PREFIX+":userId/:file", fsh.PublicDownloadHandler)
	e.DELETE("/publish/:file", fsh.DeletePublish)
	e.GET("/users", fsh.AdminUsersHandler)
	e.PATCH("/users", fsh.AdminPatchUserHandler)

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			// do some work on application stop (like closing connections and files)
			Logger.Infof("Stopping server")
			return e.Shutdown(ctx)
		},
	})

	return e
}

func configureStaticMiddleware() staticMiddleware {
	box := rice.MustFindBox("static").HTTPBox()

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			reqUrl := c.Request().RequestURI
			if reqUrl == "/" || reqUrl == "/index.html" || reqUrl == "/favicon.ico" || strings.HasPrefix(reqUrl, "/build") || strings.HasPrefix(reqUrl, "/test-assets") {
				http.FileServer(box).
					ServeHTTP(c.Response().Writer, c.Request())
				return nil
			} else {
				return next(c)
			}
		}
	}
}

func checkUrlInWhitelist(whitelist []regexp.Regexp, uri string) bool {
	for _, regexp0 := range whitelist {
		if regexp0.MatchString(uri) {
			Logger.Infof("Skipping authentication for %v because it matches %v", uri, regexp0.String())
			return true
		}
	}
	return false
}

func configureAuthMiddleware(httpClient client.RestClient) authMiddleware {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			whitelistStr := viper.GetStringSlice("auth.exclude")
			whitelist := utils.StringsToRegexpArray(whitelistStr)
			if checkUrlInWhitelist(whitelist, c.Request().RequestURI) {
				return next(c)
			}

			sessionCookie, err := c.Request().Cookie(SESSION_COOKIE)
			if err != nil {
				Logger.Infof("Error get '%v' cookie: %v", SESSION_COOKIE, err)
				return c.JSON(http.StatusUnauthorized, &utils.H{"status": "unauthorized"})
			}

			authUrl := viper.GetString(AUTH_URL)
			// check cookie
			req, err := http.NewRequest(
				"GET", authUrl, nil,
			)
			if err != nil {
				Logger.Errorf("Error during create request: %v", err)
				return err
			}

			req.AddCookie(sessionCookie)
			req.Header.Add("Accept", "application/json")
			resp, err := httpClient.Do(req)
			if err != nil {
				Logger.Errorf("Error during requesting auth backend: %v", err)
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode == 401 {
				return c.JSON(resp.StatusCode, &utils.H{"status": "unauthorized"})
			} else if resp.StatusCode == 200 {
				// put user id, user name to context
				b := resp.Body
				decoder := json.NewDecoder(b)
				var decodedResponse interface{}
				err = decoder.Decode(&decodedResponse)
				if err != nil {
					Logger.Errorf("Error during decoding json: %v", err)
					return err
				}

				dto := decodedResponse.(map[string]interface{})
				i, ok := dto["id"].(float64)
				if !ok {
					Logger.Errorf("Error during casting to int")
					return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
				}
				c.Set(utils.USER_ID, int(i))

				roles, ok2 := dto["roles"].([]interface{})
				if !ok2 {
					Logger.Errorf("Error during casting to int")
					return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
				}

				c.Set(utils.USER_ADMIN, false)
				for _, r := range roles {
					rr := r.(string)
					if len(rr) != 0 && r == viper.GetString("auth.adminRole") {
						c.Set(utils.USER_ADMIN, true)
						break
					}
				}
				c.Set(utils.USER_LOGIN, dto["login"])
				return next(c)
			} else {
				Logger.Errorf("Unknown auth status %v", resp.StatusCode)
				return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
			}

		}
	}
}

func configureMigrate(c *mongo.Client) *migrate.Migrate {
	database := utils.GetMongoDatabase(c)
	m := migrate.NewMigrate(database,
		migrate.Migration{
			Version:     1,
			Description: "drop user15",
			Up: func(db *mongo.Database) error {
				return db.Collection("user15").Drop(context.TODO())
			},
		},
		migrate.Migration{
			Version:     2,
			Description: "drop schema_migrations",
			Up: func(db *mongo.Database) error {
				return db.Collection("schema_migrations").Drop(context.TODO())
			},
		},
		migrate.Migration{
			Version:     3,
			Description: "drop user5",
			Up: func(db *mongo.Database) error {
				return db.Collection("user5").Drop(context.TODO())
			},
		},
	)
	return m
}

func runMigrate(m *migrate.Migrate) error {
	mongoClient := utils.GetMongoClient()
	defer mongoClient.Disconnect(context.TODO())

	lock := mongo_lock.NewMongoLock(mongoClient, LOCK_COLLECTION)
	lock.AcquireLock()

	if err := m.Up(migrate.AllAvailable); err != nil {
		return err
	}

	Logger.Info("Migration run successfully")

	lock.ReleaseLock()

	return nil
}

func configureMinio() *minio.Client {
	endpoint := viper.GetString("minio.endpoint")
	accessKeyID := viper.GetString("minio.accessKeyId")
	secretAccessKey := viper.GetString("minio.secretAccessKey")
	useSSL := false

	// Initialize minio client object.
	minioClient, err := minio.New(endpoint, accessKeyID, secretAccessKey, useSSL)
	if err != nil {
		Logger.Fatal(err)
	}

	return minioClient
}

func configureMongo(lc fx.Lifecycle) *mongo.Client {
	mongoClient := utils.GetMongoClient()
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			// do some work on application stop (like closing connections and files)
			Logger.Infof("Stopping mongo client")
			return mongoClient.Disconnect(ctx)
		},
	})

	return mongoClient
}

// rely on viper import and it's configured by
func runEcho(e *echo.Echo) {
	address := viper.GetString("server.address")

	Logger.Info("Starting server...")
	// Start server in another goroutine
	go func() {
		if err := e.Start(address); err != nil {
			Logger.Infof("server shut down: %v", err)
		}
	}()
	Logger.Info("Server started. Waiting for interrupt (2) (Ctrl+C)")
}

func processCli(clearMongo bool, clearMinio bool) func(mongoClient *mongo.Client, minioClient *minio.Client) error {
	return func(mongoClient *mongo.Client, minioClient *minio.Client) error {
		if clearMongo {
			Logger.Infof("Removing orphans from mongo")
			var listToDeleteFromMongo []string = make([]string, 0)
			cursor, e := utils.GetMongoDatabase(mongoClient).Collection(repository.CollectionUserFiles).Find(context.TODO(), bson.D{})
			if e != nil {
				return e
			}
			defer cursor.Close(context.TODO())
			for cursor.Next(context.TODO()) {
				var elem repository.UserFileDto
				err := cursor.Decode(&elem)
				if err != nil {
					return err
				}
				filename := elem.Id.Hex()

				found, err := searchObjectInMinio(minioClient, filename)
				if err != nil {
					return err
				}
				if !found {
					listToDeleteFromMongo = append(listToDeleteFromMongo, filename)
				}
			}
			Logger.Infof("Found %v orphans in mongo", len(listToDeleteFromMongo))

			for _, orphan := range listToDeleteFromMongo {
				Logger.Infof("Removing id='%v' from mongo", orphan)
				idDoc, e := repository.GetIdDoc(orphan)
				if e != nil {
					return e
				}
				_, e = utils.GetMongoDatabase(mongoClient).Collection(repository.CollectionUserFiles).DeleteOne(context.TODO(), idDoc)
				if e != nil {
					return e
				}
			}
		} else {
			Logger.Infof("Skipped removing orphans from mongo")
		}

		if clearMinio {
			type pair struct {
				bucketname string
				filename   string
			}

			Logger.Infof("Removing orphans from minio")
			var listToDeleteFromMinio []pair = make([]pair, 0)

			bucketInfos, err := minioClient.ListBuckets()
			if err != nil {
				return err
			}
			for _, bucketInfo := range bucketInfos {
				// Create a done channel.
				doneCh := make(chan struct{})
				defer close(doneCh)
				// Recurively list all objects in 'mytestbucket'
				recursive := true
				Logger.Debugf("Listing bucket '%v':", bucketInfo.Name)
				for objInfo := range minioClient.ListObjects(bucketInfo.Name, "", recursive, doneCh) {
					Logger.Debugf("Object '%v'", objInfo.Key)
					found, errHex, err := searchInMongo(mongoClient, objInfo.Key)
					if err != nil {
						return err
					}
					if !found || errHex != nil {
						listToDeleteFromMinio = append(listToDeleteFromMinio, pair{bucketname: bucketInfo.Name, filename: objInfo.Key})
					}
				}
			}
			Logger.Infof("Found %v orphans in minio", len(listToDeleteFromMinio))
			for _, orphan := range listToDeleteFromMinio {
				Logger.Infof("Removing '%v' from bucket '%v' of minio", orphan.filename, orphan.bucketname)
				err := minioClient.RemoveObject(orphan.bucketname, orphan.filename)
				if err != nil {
					return err
				}
			}
		} else {
			Logger.Infof("Skipped removing orphans from minio")
		}
		return nil
	}
}

func searchObjectInMinio(client3 *minio.Client, filename string) (bool, error) {
	bucketInfos, err := client3.ListBuckets()
	if err != nil {
		return false, err
	}
	for _, bucketInfo := range bucketInfos {
		// Create a done channel.
		doneCh := make(chan struct{})
		defer close(doneCh)
		// Recurively list all objects in 'mytestbucket'
		recursive := true
		Logger.Debugf("Listing bucket '%v':", bucketInfo.Name)
		for objInfo := range client3.ListObjects(bucketInfo.Name, "", recursive, doneCh) {
			Logger.Debugf("Object '%v'", objInfo.Key)
			if objInfo.Key == filename {
				return true, nil
			}
		}
	}
	return false, nil
}

func searchInMongo(mongoClient *mongo.Client, s string) (bool, error, error) {
	idDoc, e := repository.GetIdDoc(s)
	if e != nil {
		return false, e, nil
	}
	exists, e := repository.IsDocumentExists(mongoClient, repository.CollectionUserFiles, idDoc)
	if e != nil {
		return false, nil, e
	}
	return exists, nil, nil
}
