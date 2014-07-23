// Copyright (c) 2014 Genome Research Ltd.
// Author: Joshua C. Randall <jcrandall@alum.mit.edu>
//
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU General Public License as published by the Free Software
// Foundation; either version 3 of the License, or (at your option) any later
// version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS
// FOR A PARTICULAR PURPOSE. See the GNU General Public License for more
// details.
//
// You should have received a copy of the GNU General Public License along with
// this program. If not, see <http://www.gnu.org/licenses/>.
//

// hgi-vrtrack-qc-server
// exposes lanelet bamcheck files via a web service (lookup via vrpipe database)
package main

import (
	"database/sql"
	"flag"
	"fmt"
	goproperties "github.com/dmotylev/goproperties"
	"github.com/gorilla/mux"
	_ "github.com/ziutek/mymysql/godrv"
	"io"
	"log"
	"net/http"
	"os"
	"github.com/jmoiron/sqlx"
	"encoding/json"
	"encoding/xml"
)

var configFile string

func init() {
	flag.StringVar(&configFile, "config_file", "hgi-vrtrack-qc-server.conf", "Configuration file")
}

var db *sqlx.DB

func main() {
	log.Print("[hgi-vrtrack-qc-server] starting.")
	flag.Parse()

	log.Printf("[hgi-vrtrack-qc-server] loading config from %s...", configFile)
	config, err := goproperties.Load(configFile)
	if err != nil {
		log.Printf("[hgi-vrtrack-qc-server] could not load config file %s: %s", configFile, err)
	} else {
		log.Printf("[hgi-vrtrack-qc-server] loaded config.")
	}

	dbaddr := fmt.Sprintf("%s:%s:%s*%s/%s/%s", config["db.scheme"], config["db.host"], config["db.port"], config["db.name"], config["db.user"], config["db.pass"])
	log.Printf("[hgi-vrtrack-qc-server] connecting to mysql %s...", dbaddr)
	db, err = sqlx.Connect("mymysql", dbaddr)
	if err != nil {
		log.Printf("[hgi-vrtrack-qc-server] error opening SQL connection to " + dbaddr + ": " + err.Error())
		os.Exit(1)
	} else {
		log.Printf("[hgi-vrtrack-qc-server] connected.")
	}
	defer db.Close()

	r := mux.NewRouter()
	r.HandleFunc("/study/{study}", StudyHandler).Methods("GET").Name("study")
	r.HandleFunc("/", StudyHandler).Methods("GET").Name("root")

	http.Handle("/", r)
	bindaddr := config["bindaddr"]
	log.Printf("[hgi-vrtrack-qc-server] starting http listener on %s", bindaddr)
	log.Fatal(http.ListenAndServe(bindaddr, nil))
}

type StudyData struct {
     Lanelets []StudyLanelet `xml:"lanelets" json:"lanelets"`
}

type StudyLanelet struct {
     LaneletQCStatus string `db:"lanelet_qc_status" xml:"lanelet_qc_status" json:"lanelet_qc_status"`
     Individual string `db:"individual" xml:"individual" json:"individual"`
     Sample string `db:"sample" xml:"sample" json:"sample"`
     Library string `db:"library" xml:"library" json:"library"`
     Lanelet string `db:"lanelet" xml:"lanelet" json:"lanelet"`
     LaneletGTCheck string `db:"lanelet_gt_check" xml:"lanelet_gt_check" json:"lanelet_gt_check"`
     LaneletNPGQC string `db:"lanelet_npg_qc" xml:"lanelet_npg_qc" json:"lanelet_npg_qc"`
     LaneletAutoQC string `db:"lanelet_auto_qc" xml:"lanelet_auto_qc" json:"lanelet_auto_qc"`
     Readlen uint64 `db:"readlen" xml:"readlen" json:"readlen"`
     RawBasesGB float64 `db:"raw_bases_gb" xml:"raw_bases_gb" json:"raw_bases_gb"`
     MappedBasesGB float64 `db:"mapped_bases_gb" xml:"mapped_bases_gb" json:"mapped_bases_gb"`
     DuplicateReadPercent float64 `db:"duplicate_read_percent" xml:"duplicate_read_percent" json:"duplicate_read_percent"`
     MappedBasesAfterRmdupGB float64 `db:"mapped_bases_after_rmdup_gb" xml:"mapped_bases_after_rmdup_gb" json:"mapped_bases_after_rmdup_gb"`
}

func StudyHandler(w http.ResponseWriter, req *http.Request) {
	params := mux.Vars(req)
	var study string = params["study"]
	if study == "" {
		study = req.FormValue("study")
	}
	
	// TODO content negotiation (hard-code to application/json for now)
	contentType := "application/json"

	// prepare slice to hold references to each row's record
	studyLanelet := []StudyLanelet{}

	log.Printf("[StudyHandler] executing database query for study %s...\n", study)
	err := db.Select(&studyLanelet, `
	      select 
		l.qc_status as lanelet_qc_status,
		ind.name as individual,
		s.name as sample,
		lib.name as library,
		l.name as lanelet, 
		l.gt_status as lanelet_gt_check,
		l.npg_qc_status as lanelet_npg_qc,
		l.auto_qc_status as lanelet_auto_qc,
		l.readlen as readlen, 
		(l.raw_bases/1000000000) as raw_bases_gb,
		(ms.bases_mapped/1000000000) as mapped_bases_gb,
		(1-(ms.rmdup_reads_mapped/ms.reads_mapped))*100 as duplicate_read_percent,
		(ms.rmdup_bases_mapped/1000000000) as mapped_bases_after_rmdup_gb
	      from latest_lane as l
	      inner join latest_library as lib on lib.library_id = l.library_id
	      inner join latest_sample as s on s.sample_id = lib.sample_id
	      inner join individual as ind on ind.individual_id = s.individual_id
	      inner join latest_project as p on p.project_id = s.project_id
	      inner join latest_mapstats as ms on ms.lane_id = l.lane_id
	      where p.name = ?;`, study)
	switch {
	case err == sql.ErrNoRows:
		log.Printf("[StudyHandler] query returned no rows for study %s", study)
		w.WriteHeader(404)
		io.WriteString(w, fmt.Sprintf("Study %s not found", study))
	case err != nil:
		log.Printf("[StudyHandler] error executing database query: %s", err)
		w.WriteHeader(500)
		io.WriteString(w, "Error: "+err.Error())
	default:
		log.Printf("[StudyHandler] have data for study %s", study)

		studyData := StudyData{}
		studyData.Lanelets = studyLanelet

		var s []byte
		if contentType == "application/xml" {
		s, err = xml.MarshalIndent(studyData, "  ", "    ")
		if err != nil {
		  log.Printf("[StudyHandler] error marshalling study data to XML: %s", err)
		  w.WriteHeader(500)
		  io.WriteString(w, "Error: "+err.Error())
		}
} else {
		s, err = json.MarshalIndent(studyData, "  ", "    ")
		if err != nil {
		  log.Printf("[StudyHandler] error marshalling study data to JSON: %s", err)
		  w.WriteHeader(500)
		  io.WriteString(w, "Error: "+err.Error())
		}
		} 

		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(s)))
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(200)
		w.Write(s)
	}
	return
}
