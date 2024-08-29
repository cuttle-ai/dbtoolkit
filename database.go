// Copyright 2019 Cuttle.ai. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package dbtoolkit contains structures and functions required for interacting with different databases
// sub packages has the drivers for interacting with different databases
package dbtoolkit

import (
	"github.com/cuttle-ai/brain/interpreter"
	"github.com/sirupsen/logrus"
)

// Column holds the information about a column and its data type
type Column struct {
	//Name of the column
	Name string
	//DataType is the data type of the column
	DataType string
	//DateFormat is the format of the date type if column's data type is date
	DateFormat string
}

// Datastore can store data uploaded/imported by the user to cuttle platform
type Datastore interface {
	//DumpCSV will dump the given csv file to the datastore.
	//Default behaviour of the method will be to replace the existing data in the datastore.
	//But if appendData flag is set, then existing data won't be removed instead new data will be appended to it.
	DumpCSV(filename string, tablename string, columns []interpreter.ColumnNode, appendData bool, createTable bool, doScp bool, logger *logrus.Entry) error
	//DeleteTable will delete the given table in the datastore
	DeleteTable(tablename string) error
	//Exec can execute a query and return the response as the array of interfaces
	Exec(query string, args ...interface{}) ([]map[string]interface{}, error)
	//GetColumnTypes returns the list of columns and their data types for a given table
	GetColumnTypes(tableName string) ([]Column, error)
	//ChangeColumnTypeToDate changes the data type of the given column to date with the provided date format
	ChangeColumnTypeToDate(tableName string, colName string, dateFormat string) error
	//Get all the tables in the datastore
	GetTableNames() ([]string, error)
}
