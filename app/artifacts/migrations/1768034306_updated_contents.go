package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_4105455982")
		if err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(1, []byte(`{
			"autogeneratePattern": "",
			"hidden": false,
			"id": "text1404385515",
			"max": 0,
			"min": 0,
			"name": "imdb_id",
			"pattern": "",
			"presentable": false,
			"primaryKey": false,
			"required": false,
			"system": false,
			"type": "text"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(2, []byte(`{
			"autogeneratePattern": "",
			"hidden": false,
			"id": "text1438434789",
			"max": 0,
			"min": 0,
			"name": "tmdb_id",
			"pattern": "",
			"presentable": false,
			"primaryKey": false,
			"required": false,
			"system": false,
			"type": "text"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(3, []byte(`{
			"autogeneratePattern": "",
			"hidden": false,
			"id": "text724990059",
			"max": 0,
			"min": 0,
			"name": "title",
			"pattern": "",
			"presentable": false,
			"primaryKey": false,
			"required": false,
			"system": false,
			"type": "text"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(4, []byte(`{
			"hidden": false,
			"id": "select2363381545",
			"maxSelect": 2,
			"name": "type",
			"presentable": false,
			"required": false,
			"system": false,
			"type": "select",
			"values": [
				"movie",
				"serie"
			]
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(5, []byte(`{
			"hidden": false,
			"id": "number4134912800",
			"max": null,
			"min": null,
			"name": "release_year",
			"onlyInt": false,
			"presentable": false,
			"required": false,
			"system": false,
			"type": "number"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(6, []byte(`{
			"hidden": false,
			"id": "number2254405824",
			"max": null,
			"min": null,
			"name": "duration",
			"onlyInt": false,
			"presentable": false,
			"required": false,
			"system": false,
			"type": "number"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(7, []byte(`{
			"autogeneratePattern": "",
			"hidden": false,
			"id": "text3199963017",
			"max": 0,
			"min": 0,
			"name": "plot",
			"pattern": "",
			"presentable": false,
			"primaryKey": false,
			"required": false,
			"system": false,
			"type": "text"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(8, []byte(`{
			"hidden": false,
			"id": "number1267793696",
			"max": null,
			"min": null,
			"name": "imdb_rating",
			"onlyInt": false,
			"presentable": false,
			"required": false,
			"system": false,
			"type": "number"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(9, []byte(`{
			"hidden": false,
			"id": "number3568314025",
			"max": null,
			"min": null,
			"name": "vote_count",
			"onlyInt": false,
			"presentable": false,
			"required": false,
			"system": false,
			"type": "number"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(10, []byte(`{
			"autogeneratePattern": "",
			"hidden": false,
			"id": "text2092043024",
			"max": 0,
			"min": 0,
			"name": "quality",
			"pattern": "",
			"presentable": false,
			"primaryKey": false,
			"required": false,
			"system": false,
			"type": "text"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(11, []byte(`{
			"autogeneratePattern": "",
			"hidden": false,
			"id": "text4019417054",
			"max": 0,
			"min": 0,
			"name": "poster_url",
			"pattern": "",
			"presentable": false,
			"primaryKey": false,
			"required": false,
			"system": false,
			"type": "text"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(12, []byte(`{
			"hidden": false,
			"id": "number2023681770",
			"max": null,
			"min": null,
			"name": "poster_width",
			"onlyInt": false,
			"presentable": false,
			"required": false,
			"system": false,
			"type": "number"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(13, []byte(`{
			"hidden": false,
			"id": "number515331995",
			"max": null,
			"min": null,
			"name": "poster_height",
			"onlyInt": false,
			"presentable": false,
			"required": false,
			"system": false,
			"type": "number"
		}`)); err != nil {
			return err
		}

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_4105455982")
		if err != nil {
			return err
		}

		// remove field
		collection.Fields.RemoveById("text1404385515")

		// remove field
		collection.Fields.RemoveById("text1438434789")

		// remove field
		collection.Fields.RemoveById("text724990059")

		// remove field
		collection.Fields.RemoveById("select2363381545")

		// remove field
		collection.Fields.RemoveById("number4134912800")

		// remove field
		collection.Fields.RemoveById("number2254405824")

		// remove field
		collection.Fields.RemoveById("text3199963017")

		// remove field
		collection.Fields.RemoveById("number1267793696")

		// remove field
		collection.Fields.RemoveById("number3568314025")

		// remove field
		collection.Fields.RemoveById("text2092043024")

		// remove field
		collection.Fields.RemoveById("text4019417054")

		// remove field
		collection.Fields.RemoveById("number2023681770")

		// remove field
		collection.Fields.RemoveById("number515331995")

		return app.Save(collection)
	})
}
