package mongo_lock

import (
	"context"
	"fmt"
	"github.com/labstack/gommon/log"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/nkonev/blog-store/utils"
	"github.com/stretchr/testify/assert"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	setup()
	retCode := m.Run()
	shutdown()
	os.Exit(retCode)
}

func shutdown() {

}

func setup() {
	utils.InitViper("../config-dev/config.yml")

	log.Info("Set up")
	utils.DropMongo()
}

func TestHangsOnLocked(t *testing.T) {
	mongoClient := utils.GetMongoClient()
	defer mongoClient.Disconnect(context.TODO())

	testLock := "test_locked_1"
	coll := mongoClient.Database(utils.GetMongoDbName(utils.GetMongoUrl())).Collection(testLock)
	result, err := coll.InsertOne(context.TODO(), GetIdDoc())
	if err != nil {
		log.Fatalf("error during insert: %v", err)
	}
	log.Infof("InsertId %v", result.InsertedID)

	lock := NewMongoLock(mongoClient, testLock)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		lock.AcquireLock()
	}()
	go func() {
		defer wg.Done()
		time.Sleep(time.Second * 4)
		coll.DeleteOne(context.TODO(), GetIdDoc())
	}()

	wg.Wait()
}

func hasIndex(coll *mongo.Collection, indexName string) bool {
	cursor, e := coll.Indexes().List(context.TODO())
	defer cursor.Close(context.TODO())
	if e != nil {
		log.Fatalf("error during listing indexes: %v", e)
	}
	var hasUniqueIndex bool
	for cursor.Next(context.TODO()) {
		bytes, e := cursor.DecodeBytes()
		str := fmt.Sprintf("%v", bytes)
		if strings.Contains(str, indexName) {
			hasUniqueIndex = true
			break
		}
		log.Infof("Indexes %v %v", str, e)
	}
	return hasUniqueIndex
}

func TestLockIsReleased(t *testing.T) {
	mongoClient := utils.GetMongoClient()
	defer mongoClient.Disconnect(context.TODO())

	testLock := "test_locked_2"
	coll := mongoClient.Database(utils.GetMongoDbName(utils.GetMongoUrl())).Collection(testLock)

	lock := NewMongoLock(mongoClient, testLock)

	assert.False(t, hasIndex(coll, "unique_id"))

	lock.AcquireLock()

	assert.True(t, hasIndex(coll, "unique_id"))

	one := coll.FindOne(context.TODO(), GetIdDoc())
	raws, err := one.DecodeBytes()
	if err != nil {
		log.Fatalf("error during find: %v", err)
	}
	assert.NotNil(t, raws)

	lock.ReleaseLock()

	oneAfter := coll.FindOne(context.TODO(), GetIdDoc())
	rawsAfter, errAfter := oneAfter.DecodeBytes()
	assert.NotNil(t, errAfter)
	assert.Nil(t, rawsAfter)
}
