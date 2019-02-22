package mongo_lock

import (
	"context"
	"github.com/nkonev/blog-store/.vendor-new/github.com/labstack/gommon/log"
	"github.com/nkonev/blog-store/utils"
	"os"
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

func TestLockIsReleased(t *testing.T) {
}
