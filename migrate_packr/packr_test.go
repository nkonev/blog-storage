package migrate_packr

import (
	rice "github.com/GeertJohan/go.rice"
	st "github.com/golang-migrate/migrate/v4/source/testing"
	log "github.com/sirupsen/logrus"
	"testing"
)

func Test(t *testing.T) {
	box := rice.MustFindBox("testdata")

	d, err := WithInstance(box)
	if err != nil {
		log.Panicf("Error during create migrator driver: %v", err)
	}

	st.Test(t, d)
}

func TestOpen(t *testing.T) {
	p := &Packr{}
	_, err := p.Open("")
	if err == nil {
		t.Fatal("expected err, because it's not implemented yet")
	}
}
