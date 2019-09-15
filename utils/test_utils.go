package utils

import (
	"context"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

func DropMongo() {
	mongoUrl := GetMongoUrl()
	client := GetMongoClient()
	defer client.Disconnect(context.TODO())
	uri, err := connstring.Parse(mongoUrl)
	if err != nil {
		log.Panicf("Error during parsing url: '%v'", err)
	}
	err = client.Database(uri.Database).Drop(context.Background())
	if err != nil {
		log.Panicf("Error during dropping database: '%v'", err)
	}
	log.Infof("Mongo database '%v' successfully dropped", uri.Database)
}
