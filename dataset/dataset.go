// Copyright 2019 Cuttle.ai. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package dataset has the utilities to make the datasets efficient.
// It has utilities like post processing to identify the dimensions in the dataset
// To identify the date columns
package dataset

import (
	"strconv"

	"github.com/cuttle-ai/brain/interpreter" // making comments
	"github.com/cuttle-ai/brain/models"
	"github.com/cuttle-ai/dbtoolkit"
	"github.com/cuttle-ai/dbtoolkit/datastores/services"
	"github.com/jinzhu/gorm"
	"github.com/sirupsen/logrus"
)

// IdentifyDimensions will identify the dimensions in a given dataset and update the same in the db
func IdentifyDimensions(l *logrus.Entry, conn *gorm.DB, cols []models.Node, table models.Node, dSer services.Service, dt *models.Dataset) error {
	/*
	 * we will first get the columns that are not of the type dimension
	 * We will get the datastore in which the table is stored in
	 * Then we will iterate through the columns
	 *		and check the number of unique values the column is holding.
	 * 		based that we will update the dimension type
	 * For the columns with udpated dimension type, we will update them in the db
	 */
	//filter columns of not type dimension
	columns := []interpreter.ColumnNode{}
	colMap := map[string]models.Node{}
	for _, v := range cols {
		colMap[v.UID.String()] = v
		iN := v.ColumnNode()
		if !iN.Dimension {
			columns = append(columns, iN)
		}
	}
	if len(columns) == 0 {
		return nil
	}

	//getting the datastore in which the table is stored in
	dStore, err := dSer.Datastore()
	if err != nil {
		//error while getting the datastore
		l.Error("error while getting the datastore", dSer.ID)
		return err
	}

	//iterating through the columns to identify whether they are of type dimension
	dimCols := []models.Node{}
	tb := table.TableNode()
	for i := 0; i < len(columns); i++ {
		//get the unique values the columns are holding
		result, err := dStore.Exec("SELECT COUNT(DISTINCT(\"" + columns[i].Name + "\")) FROM \"" + tb.Name + "\"")
		if err != nil {
			//error while querying the datastore to find the count of the unique values in the column
			l.Error("error while querying the datastore to find the count of the unique values in the column", columns[i].Name, "from the table", tb.Name)
			l.Error(err)
			continue
		}
		count := 0
		for _, row := range result {
			for _, val := range row {
				str, _ := val.(*string)
				count, _ = strconv.Atoi(*str)
			}
		}

		//update the dimension type if required
		if count > 0 && count < 50 {
			columns[i].Dimension = true
			c, _ := colMap[columns[i].UID]
			dimCols = append(dimCols, c.FromColumn(columns[i]))
		}
	}
	l.Info("have got", len(dimCols), "columns that are of type dimension")

	//for the updated columns, update the info in the db
	_, err = dt.UpdateColumns(l, conn, dimCols)
	if err != nil {
		//error while updating the dimension status of the columns
		l.Error("error while updating the dimension status of the columns of the table", tb.Name)
		return err
	}

	return nil
}

// ConvertDates will identify the dates in the datsets and update the same in the db
func ConvertDates(l *logrus.Entry, conn *gorm.DB, cols []models.Node, table models.Node, dSer services.Service, dt *models.Dataset) error {
	/*
	 * We will first get the columns having date data type
	 * We will get the datastore in which the table is stored in
	 * Then we will get the datatype of the columns in db
	 * If the data type in db is different, then we will change the data type
	 * The we will update the table's default date field if not available with one
	 */
	//getting the columns having the date data type
	tN := table.TableNode()
	l.Info("going to convert the date columns in the dataset from string to date", tN.Name)
	columns := []interpreter.ColumnNode{}
	colMap := map[string]interpreter.ColumnNode{}
	for _, v := range cols {
		iN := v.ColumnNode()
		colMap[iN.Name] = iN
		if iN.DataType == interpreter.DataTypeDate {
			columns = append(columns, iN)
		}
	}
	if len(columns) == 0 {
		return nil
	}

	l.Info("have", len(columns), "to that are of date type in", tN.Name)
	//getting the datastore
	dStore, err := dSer.Datastore()
	if err != nil {
		//error while getting the datastore
		l.Error("error while getting the datastore", dSer.ID)
		return err
	}

	//getting the column data types
	colTypes, err := dStore.GetColumnTypes(tN.Name)
	if err != nil {
		//error while getting the column data types
		l.Error("error while getting the datatypes of the columns of tha table", tN.Name)
		return err
	}

	//checking the cols with different data type
	toBeChanged := []dbtoolkit.Column{}
	for _, v := range colTypes {
		colNode, ok := colMap[v.Name]
		if !ok {
			continue
		}
		if colNode.DataType == interpreter.DataTypeDate && v.DataType == interpreter.DataTypeString {
			v.DateFormat = colNode.DateFormat
			v.DataType = interpreter.DataTypeDate
			toBeChanged = append(toBeChanged, v)
		}
	}
	l.Info(len(toBeChanged), "columns havbe the datatypes to be changed from text to date in", tN.Name)

	//if the len of columns to be changed is zero, don't go forward
	if len(toBeChanged) == 0 {
		return nil
	}
	for _, v := range toBeChanged {
		err := dStore.ChangeColumnTypeToDate(tN.Name, v.Name, v.DateFormat)
		if err != nil {
			//error while converting the data type of the column
			l.Error("error while converting the data type to date for the column", v.Name, tN.Name)
			return err
		}
	}
	l.Info("altered the column types from text to date", tN.Name)

	//now we update the first column as default date
	if len(tN.DefaultDateFieldUID) != 0 {
		//we already have a default date field
		return nil
	}
	colNode := colMap[toBeChanged[0].Name]
	l.Info("updating the default date column of the table", tN.Name, "to", colNode.Name)
	tN.DefaultDateField = &colNode
	tN.DefaultDateFieldUID = colNode.UID
	_, err = dt.UpdateTable(conn, table.FromTable(tN))
	if err != nil {
		//error while updating the table with default date
		l.Error("error while updating the table with default date", colNode.Name, tN.Name)
		return err
	}

	return nil
}

// OptimizeDatasetMetadata will optimize metadata associated with a dataset
// It will find the dimension columns in the dataset
// It will also try to identify the date columns in the datasets
func OptimizeDatasetMetadata(l *logrus.Entry, conn *gorm.DB, id uint, dSer services.Service, userID uint) error {
	/*
	 * We will get the dataset info from the db
	 * Then we will get the columns in the dataset
	 * Then we will get the table in the dataset
	 * Then we will identify the dimensions in the dataset
	 * Then we will convert the dates in the dataset
	 */
	dt := &models.Dataset{Model: gorm.Model{ID: id}, UserID: userID}
	l.Info("going to optimize the dataset metadata for", dt.ID)

	//getting the dataset info from the database
	err := dt.Get(conn)
	if err != nil {
		//error while finding the dataset info from the db
		l.Error("error while finding the dataset info from the db")
		return err
	}

	//get the columns in the dataset
	cols, err := dt.GetColumns(conn)
	if err != nil {
		//error while finding the columns in the dataset
		l.Error("error while finding the columns in the dataset from the db")
		return err
	}

	//get the table in the dataset
	table, err := dt.GetTable(conn)
	if err != nil {
		//error while finding the table in the dataset
		l.Error("error while finding the table in the dataset from the db")
		return err
	}

	//identify the dimensions in the dataset
	err = IdentifyDimensions(l, conn, cols, table, dSer, dt)
	if err != nil {
		//error while identifying the dimension columns in the dataset
		l.Error("error while identifying the dimension columns in the dataset")
		return err
	}

	//convert the dates in the dataset
	err = ConvertDates(l, conn, cols, table, dSer, dt)
	if err != nil {
		//error while converting the date columns in the dataset
		l.Error("error while converting the date columns in the dataset")
		return err
	}

	l.Info("successfully optimized the dataset metadata for", dt.ID)
	return nil
}
