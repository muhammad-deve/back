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
			"id": "select3130199401",
			"maxSelect": 2,
			"name": "profession",
			"presentable": false,
			"required": false,
			"system": false,
			"type": "select",
			"values": [
				"actor",
				"writer",
				"director"
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
			"id": "select3130199401",
			"maxSelect": 2,
			"name": "profession",
			"presentable": false,
			"required": false,
			"system": false,
			"type": "select",
			"values": [
				"actor",
				"writer",
				"scenarist",
				"director"
			]
		}`)); err != nil {
			return err
		}

		return app.Save(collection)
	})
}
