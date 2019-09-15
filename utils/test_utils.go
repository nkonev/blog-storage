package utils

import (
	"context"
	"github.com/mongodb/mongo-go-driver/x/network/connstring"
	log "github.com/sirupsen/logrus"
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
