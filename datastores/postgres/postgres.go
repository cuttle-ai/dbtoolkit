// Copyright 2019 Cuttle.ai. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package postgres has the datastore implementation for the postgres database to store data in cuttle platform
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	//this package contains the postgres driver for cuttle to use it as a datastore. That is why the initalization done here
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"

	"github.com/cuttle-ai/brain/interpreter"
	"github.com/cuttle-ai/dbtoolkit"
)

// Postgres is the postgre datastore
type Postgres struct {
	//DB connection instance
	DB *sql.DB
	//DataDumpDirectory will be the name of the directory with the user name attached to it
	//Eg. user@myserver.com:/home/user/data-directory
	DataDumpDirectory string
}

// NewPostgres returns the postgres with active connection
func NewPostgres(host, port, dbName, username, password, dataDumpDirectory string) (*Postgres, error) {
	cStr := fmt.Sprintf("host=%s port=%s dbname='%s'  user=%s password=%s sslmode=disable",
		host, port, dbName, username, password)
	db, err := sql.Open("postgres", cStr)
	if err != nil {
		return nil, err
	}
	return &Postgres{DB: db, DataDumpDirectory: dataDumpDirectory}, nil
}

func convertToPostgresDataType(dataType string, maskDate bool) string {
	switch dataType {
	case interpreter.DataTypeString:
		return "text"
	case interpreter.DataTypeFloat:
		return "float"
	case interpreter.DataTypeInt:
		return "int"
	case interpreter.DataTypeDate:
		if maskDate {
			return "text"
		}
		return "date"
	default:
		return "text"
	}
}

func convertFromPostgresDataType(dataType string) string {
	switch dataType {
	case "text":
		return interpreter.DataTypeString
	case "float":
		return interpreter.DataTypeFloat
	case "int":
		return interpreter.DataTypeInt
	case "date":
		return interpreter.DataTypeDate
	default:
		return interpreter.DataTypeString
	}
}

// DumpCSV will dump the given csv file to post instance
func (p Postgres) DumpCSV(filename string, tablename string, columns []interpreter.ColumnNode, appendData bool, createTable bool, doScp bool, logger *logrus.Entry) error {
	/*
	 * We will copy the file to the remote
	 * We will start a transaction for the db operation
	 * Then we will create the table required
	 * If required remove the existing data
	 * Then we will dump the data to the datastore
	 * Then we will remove the file from the remote
	 */
	//copying the file to the remote
	logger.Info("copying the file to remote postgres server", p.DataDumpDirectory)
	cmdName := "cp"
	args := []string{filename, p.DataDumpDirectory + "/" + tablename + ".csv"}
	if doScp {
		cmdName = "scp"
		args = []string{"-o", "StrictHostKeyChecking=no", filename, p.DataDumpDirectory + "/" + tablename + ".csv"}
	}
	cm := exec.Command(cmdName, args...)
	err := cm.Run()
	if err != nil {
		logger.Error("error copying the file for dumping csv to the datastore", filename, "to", p.DataDumpDirectory)
		return err
	}

	//starting the db transaction
	tx, err := p.DB.BeginTx(context.Background(), nil)
	if err != nil {
		logger.Error("error while creating the db transaction for dumping csv to the datastore")
		return err
	}

	//we will create the table
	//we will first build the query string to create the table
	logger.Info("building the table to dump the csv", tablename)
	var strB strings.Builder
	var strC strings.Builder
	strB.WriteString("CREATE TABLE ")
	strB.WriteString(`"` + tablename + `"`)
	strB.WriteString("( ")
	strC.WriteString("(")
	for k, col := range columns {
		if k > 0 {
			strB.WriteString(", ")
			strC.WriteString(", ")
		}
		strB.WriteString("\"" + col.Name + "\" " + convertToPostgresDataType(col.DataType, true))
		strC.WriteString("\"" + col.Name + "\"")
	}
	strB.WriteString(" )")
	strC.WriteString(" )")
	//now executing the built query

	if createTable {
		_, err = tx.Exec(strB.String())
		if err != nil {
			logger.Error("error while creating the table", tablename, "for dumping the csv data to the datastore")
			return err
		}
	}

	if !appendData && !createTable {
		_, err = tx.Exec("TRUNCATE TABLE \"" + tablename + "\"")
		if err != nil {
			logger.Error("error while truncating the table", tablename, "for replacing the csv data in the datastore")
			return err
		}
	}

	//now we will dump the data to the datastore
	fileNameSplitted := strings.Split(filename, string([]rune{filepath.Separator}))
	if len(fileNameSplitted) < 1 {
		logger.Error("expected the filename to have atleast 1 part. got", len(fileNameSplitted))
		return errors.New("expected the filename to have atleast 1 part. got " + strconv.Itoa(len(fileNameSplitted)))
	}
	remoteFileName := p.DataDumpDirectory + "/" + tablename + ".csv"
	remoteFileNameSplitted := strings.Split(remoteFileName, ":")
	if len(remoteFileNameSplitted) < 2 {
		logger.Error("expected the filename to have server info like user@server.com:/home/user. Couldn't find one", remoteFileName)
		return errors.New("expected the filename to have server info like user@server.com:/home/user. Couldn't find one + remoteFileName")
	}
	remoteFileNameWithoutServer := remoteFileNameSplitted[len(remoteFileNameSplitted)-1]

	logger.Info("copying the data from the csv to the table", remoteFileNameWithoutServer, tablename)
	qStr := fmt.Sprintf(`COPY "%s" %s FROM '%s' DELIMITER ',' CSV HEADER;`, tablename, strC.String(), remoteFileNameWithoutServer)
	result, err := tx.Exec(qStr)
	if err != nil {
		logger.Error("error while dumping to the table", tablename, "from csv", remoteFileNameWithoutServer)
		return err
	}
	ef, err := result.RowsAffected()
	if err != nil {
		logger.Error("error while getting the number of rows affected while dumping the data to the datastore")
		return err
	}

	err = tx.Commit()
	if err != nil {
		logger.Error("error while commiting the changes")
		return err
	}

	//removing the file from remote
	logger.Info("removing the data file from the remote server")
	rmCmd := exec.Command("ssh", remoteFileNameSplitted[0], "rm", remoteFileNameWithoutServer)
	err = rmCmd.Run()
	if err != nil {
		logger.Error("error removing the file from the server after dumping csv to the datastore", remoteFileName)
		return err
	}

	logger.Info("successfully dumped the csv to the table", filename, tablename, "copied no. of rows:-", ef)
	return nil
}

// DeleteTable deletes the table from the datastore
func (p Postgres) DeleteTable(tablename string) error {
	_, err := p.DB.Exec("drop table \"" + tablename + "\"")
	return err
}

// Exec will execute a query in the post gres
func (p Postgres) Exec(query string, args ...interface{}) ([]map[string]interface{}, error) {
	/*
	 * We will add a db check
	 * Then we will query the datastore
	 * Then will iterate through the results and parse them
	 * Finally check the erros and return
	 */

	//db check
	if p.DB == nil {
		//couldn't connect to the postgres since no connection available
		return nil, errors.New("couldn't find the datastore connection to the postgres")
	}

	//datastore query
	rows, err := p.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	//iterating through the resutls and parsing the same
	results := []map[string]interface{}{}
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		result := map[string]interface{}{}
		vals := make([]interface{}, len(cols))
		for i := 0; i < len(vals); i++ {
			v := ""
			vals[i] = &v
		}
		if err := rows.Scan(vals...); err != nil {
			return nil, err
		}
		for i, v := range vals {
			result[cols[i]] = v
		}
		results = append(results, result)
	}

	// Check for errors from iterating over rows.
	if err := rows.Err(); err != nil {
		return results, err
	}
	return results, nil
}

// GetColumnTypes returns the column types of the given table name
func (p Postgres) GetColumnTypes(tableName string) ([]dbtoolkit.Column, error) {
	rows, err := p.DB.Query("SELECT column_name, data_type FROM information_schema.columns WHERE table_name = '" + tableName + "'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []dbtoolkit.Column{}
	for rows.Next() {
		vals := make([]interface{}, 2)
		for i := 0; i < len(vals); i++ {
			v := ""
			vals[i] = &v
		}
		if err := rows.Scan(vals...); err != nil {
			return nil, err
		}
		name, _ := vals[0].(*string)
		dataType, _ := vals[1].(*string)
		results = append(results, dbtoolkit.Column{Name: *name, DataType: convertFromPostgresDataType(*dataType)})
	}
	return results, nil
}

func (p Postgres) GetTableNames() ([]string, error) {
	rows, err := p.DB.Query("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []string{}
	for rows.Next() {
		vals := make([]interface{}, 1)
		for i := 0; i < len(vals); i++ {
			v := ""
			vals[i] = &v
		}
		if err := rows.Scan(vals...); err != nil {
			return nil, err
		}
		tableName, ok := vals[0].(*string)
		if !ok {
			return nil, errors.New("couldn't parse the table name from the query results")
		}
		results = append(results, *tableName)
	}
	return results, nil
}

// ChangeColumnTypeToDate changes a given column's data type to date with the date format as provided
func (p Postgres) ChangeColumnTypeToDate(tableName string, colName string, dateFormat string) error {
	_, err := p.DB.Exec("ALTER TABLE \"" + tableName + "\" ALTER COLUMN \"" + colName + "\" TYPE DATE using to_date(\"" + colName + "\", '" + convertToPostgresFormat(dateFormat) + "')")
	return err
}

func (p Postgres) Close() error {
	return p.DB.Close()
}

// Ping the datastore
func (p Postgres) Ping() error {
	return p.DB.Ping()
}

func convertToPostgresFormat(dateFormat string) string {
	convertedDateFormat := strings.Replace(dateFormat, "2006", "YYYY", 1)
	convertedDateFormat = strings.Replace(convertedDateFormat, "1", "mm", 1)
	convertedDateFormat = strings.Replace(convertedDateFormat, "2", "dd", 1)
	convertedDateFormat = strings.Replace(convertedDateFormat, "Jan", "Mon", 1)
	fmt.Println(convertedDateFormat)
	return convertedDateFormat
}
