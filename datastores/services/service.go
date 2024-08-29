// Copyright 2019 Cuttle.ai. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package services has the service models for storing the datastore service info
package services

import (
	"errors"
	"strconv"

	"github.com/cuttle-ai/dbtoolkit"
	"github.com/cuttle-ai/dbtoolkit/datastores/postgres"
	"github.com/jinzhu/gorm"
)

const (
	//POSTGRES represents the postgres type of datastore service
	POSTGRES = "POSTGRES"
)

// Service is defnition of the datastore service
type Service struct {
	gorm.Model
	//URL at which the service is available
	URL string
	//Port is the port at which the datastore service is available
	Port string
	//Username for authentication with the datastore service
	Username string
	//Password for authentication with the datastore service
	Password string
	//Name is the name of the datastore db
	Name string
	//Group to which the datastore belongs to classify the service
	Group string
	//Datasets has the number of datasets stored in the database
	Datasets int
	//DatastoreType indicates the type of datastore like postgres etc
	DatastoreType string
	//DataDirectory is the directory where the data is stored
	DataDirectory string
}

// GetAll returns the list of datastore available
func GetAll(conn *gorm.DB) ([]Service, error) {
	result := []Service{}
	err := conn.Find(&result).Error
	return result, err
}

// Get will set the info of the service for the give id from database
func (s *Service) Get(conn *gorm.DB) error {
	return conn.Find(s).Error
}

// Validate validates whether the given service is valid or not
func (s Service) Validate() error {
	if len(s.URL) == 0 {
		return errors.New("URL can't be empty")
	}
	if len(s.Port) == 0 {
		return errors.New("Port can't be empty")
	}
	_, err := strconv.ParseInt(s.Port, 10, 32)
	if err != nil {
		return errors.New("Port should be a valid number got " + s.Port)
	}
	if len(s.Username) == 0 {
		return errors.New("Username can't be empty")
	}
	if len(s.Password) == 0 {
		return errors.New("Password can't be empty")
	}
	if len(s.Name) == 0 {
		return errors.New("Name can't be empty")
	}
	if len(s.Group) == 0 {
		return errors.New("Group can't be empty")
	}
	if len(s.DatastoreType) == 0 {
		return errors.New("Type can't be empty")
	}
	if len(s.DataDirectory) == 0 {
		return errors.New("Data Directory can't be empty")
	}
	return nil
}

// Create will create a given service
func (s *Service) Create(conn *gorm.DB) error {
	return conn.Create(s).Error
}

// Update will update a given service
func (s *Service) Update(conn *gorm.DB) error {
	return conn.Model(s).Updates(map[string]interface{}{
		"url":            s.URL,
		"port":           s.Port,
		"username":       s.Username,
		"password":       s.Password,
		"name":           s.Name,
		"group":          s.Group,
		"datastore_type": s.DatastoreType,
		"data_directory": s.DataDirectory,
	}).Error
}

// AddDataset will add 1 to the datasets count of the service
func (s *Service) AddDataset(conn *gorm.DB) error {
	return conn.Model(s).Updates(map[string]interface{}{
		"datasets": s.Datasets + 1,
	}).Error
}

// RemoveDataset will remove 1 from the datasets count of the service
func (s *Service) RemoveDataset(conn *gorm.DB) error {
	return conn.Model(s).Updates(map[string]interface{}{
		"datasets": s.Datasets - 1,
	}).Error
}

// Delete will delete a given service
func (s *Service) Delete(conn *gorm.DB) error {
	return conn.Delete(s).Error
}

// Datastore returns the datastore associated with a service. It iwll return nil and boolean as false if the service doesn't represent a correct datastore
func (s Service) Datastore() (dbtoolkit.Datastore, error) {
	if s.DatastoreType == POSTGRES {
		ps, err := postgres.NewPostgres(s.URL, s.Port, s.Name, s.Username, s.Password, s.DataDirectory)
		if err != nil {
			//error while creating a postgres connection
			return nil, err
		}
		return ps, nil
	}
	return nil, errors.New("couldn't identify the type of service " + s.DatastoreType)
}
