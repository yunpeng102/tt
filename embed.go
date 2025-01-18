package main

import (
	_ "embed"
)

//go:embed init_db.sql
var initDBSQL string
