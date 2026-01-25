package repository

import (
	"github.com/jmoiron/sqlx"
)

type Repositories struct {
	db *sqlx.DB
	//MeteringPointRepo *MeteringPointRepository
	//ParticipantRepo   *ParticipantRepository
	//EegRepo           *EegRepository
}

//var DbRepos *Repositories

func InitRepositories() {

	//db, err := database.ConnectToDatabase()
	//if err != nil {
	//	panic(100)
	//}
	//
	//DbRepos = &Repositories{
	//	db:                db,
	//	MeteringPointRepo: &MeteringPointRepository{db: db},
	//	ParticipantRepo:   &ParticipantRepository{db: db, dialect: goqu.Dialect("postgres")},
	//	EegRepo:           &EegRepository{db: db},
	//}
}

//func CloseRepositories() {
//	if DbRepos.db != nil {
//		_ = DbRepos.db.Close()
//	}
//}
