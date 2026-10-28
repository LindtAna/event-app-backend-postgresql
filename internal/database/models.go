package database

import "database/sql"

type Models struct {
	Tenants     TenantModel
	Users       UserModel
	Events      EventModel
	Attendees   AttendeeModel
	Categories  CategoryModel
	Invitations InvitationModel
}

func NewModels(db *sql.DB) Models {
	return Models{
		Users:       UserModel{DB: db},
		Tenants:     TenantModel{DB: db},
		Events:      EventModel{DB: db},
		Attendees:   AttendeeModel{DB: db},
		Categories:  CategoryModel{DB: db},
		Invitations: InvitationModel{DB: db},
	}
}
