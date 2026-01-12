package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_3009067695")
		if err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(5, []byte(`{
			"cascadeDelete": false,
			"collectionId": "pbc_961350965",
			"hidden": false,
			"id": "relation1400097126",
			"maxSelect": 1,
			"minSelect": 0,
			"name": "country",
			"presentable": false,
			"required": false,
			"system": false,
			"type": "relation"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(6, []byte(`{
			"cascadeDelete": false,
			"collectionId": "pbc_3304764897",
			"hidden": false,
			"id": "relation3571151285",
			"maxSelect": 1,
			"minSelect": 0,
			"name": "language",
			"presentable": false,
			"required": false,
			"system": false,
			"type": "relation"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(7, []byte(`{
			"cascadeDelete": false,
			"collectionId": "pbc_1410514596",
			"hidden": false,
			"id": "relation3834550803",
			"maxSelect": 1,
			"minSelect": 0,
			"name": "logo",
			"presentable": false,
			"required": false,
			"system": false,
			"type": "relation"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(8, []byte(`{
			"cascadeDelete": false,
			"collectionId": "pbc_3292755704",
			"hidden": false,
			"id": "relation105650625",
			"maxSelect": 1,
			"minSelect": 0,
			"name": "category",
			"presentable": false,
			"required": false,
			"system": false,
			"type": "relation"
		}`)); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(9, []byte(`{
			"hidden": false,
			"id": "bool1537641292",
			"name": "is_working",
			"presentable": false,
			"required": false,
			"system": false,
			"type": "bool"
		}`)); err != nil {
			return err
		}

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_3009067695")
		if err != nil {
			return err
		}

		// remove field
		collection.Fields.RemoveById("relation1400097126")

		// remove field
		collection.Fields.RemoveById("relation3571151285")

		// remove field
		collection.Fields.RemoveById("relation3834550803")

		// remove field
		collection.Fields.RemoveById("relation105650625")

		// remove field
		collection.Fields.RemoveById("bool1537641292")

		return app.Save(collection)
	})
}
