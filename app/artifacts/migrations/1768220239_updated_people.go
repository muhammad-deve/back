package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_520427368")
		if err != nil {
			return err
		}

		// update field
		if err := collection.Fields.AddMarshaledJSONAt(4, []byte(`{
			"hidden": false,
			"id": "select802850298",
			"maxSelect": 3,
			"name": "professions",
			"presentable": false,
			"required": false,
			"system": false,
			"type": "select",
			"values": [
				"actor",
				"producer",
				"director",
				"actress"
			]
		}`)); err != nil {
			return err
		}

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_520427368")
		if err != nil {
			return err
		}

		// update field
		if err := collection.Fields.AddMarshaledJSONAt(4, []byte(`{
			"hidden": false,
			"id": "select802850298",
			"maxSelect": 3,
			"name": "professions",
			"presentable": false,
			"required": false,
			"system": false,
			"type": "select",
			"values": [
				"actor",
				"producer",
				"director"
			]
		}`)); err != nil {
			return err
		}

		return app.Save(collection)
	})
}
