package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_4044198014")
		if err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(4, []byte(`{
			"exceptDomains": null,
			"hidden": false,
			"id": "url2602326153",
			"name": "url_imdb",
			"onlyDomains": null,
			"presentable": false,
			"required": false,
			"system": false,
			"type": "url"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(5, []byte(`{
			"exceptDomains": null,
			"hidden": false,
			"id": "url963631051",
			"name": "url_tmdb",
			"onlyDomains": null,
			"presentable": false,
			"required": false,
			"system": false,
			"type": "url"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(6, []byte(`{
			"exceptDomains": null,
			"hidden": false,
			"id": "url2221138649",
			"name": "img_url",
			"onlyDomains": null,
			"presentable": false,
			"required": false,
			"system": false,
			"type": "url"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(7, []byte(`{
			"cascadeDelete": false,
			"collectionId": "pbc_893542168",
			"hidden": false,
			"id": "relation2092043024",
			"maxSelect": 1,
			"minSelect": 0,
			"name": "quality",
			"presentable": false,
			"required": false,
			"system": false,
			"type": "relation"
		}`)); err != nil {
			return err
		}

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_4044198014")
		if err != nil {
			return err
		}

		// remove field
		collection.Fields.RemoveById("url2602326153")

		// remove field
		collection.Fields.RemoveById("url963631051")

		// remove field
		collection.Fields.RemoveById("url2221138649")

		// remove field
		collection.Fields.RemoveById("relation2092043024")

		return app.Save(collection)
	})
}
