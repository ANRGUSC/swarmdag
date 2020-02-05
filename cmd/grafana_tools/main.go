package main

/*
   Copyright 2016 Alexander I.Grafov <grafov@gmail.com>
   Copyright 2016-2019 The Grafana SDK authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.

   ॐ तारे तुत्तारे तुरे स्व
*/

import (
	"log"
	"strings"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/gosimple/slug"
	"github.com/grafana-tools/sdk"
)

func main() {
    var (
        datasources []sdk.Datasource
        dsPacked    []byte
        meta        sdk.BoardProperties
        err         error

        boardLinks []sdk.FoundBoard
        rawBoard   []byte

        filesInDir  []os.FileInfo
        rawDS       []byte
        status      sdk.StatusMessage
    )
    if len(os.Args) != 4 {
        fmt.Fprint(os.Stderr, "Usage:  ./grafana-tools [commmand] http://sdk.host:3000 <username:pw OR api-key>\n" +
                   "\ncommands: 'import-datasource', 'import-dashboard', 'backup-datasource', or 'backup-dashboard'\n")
        os.Exit(0)
    }
    
    c := sdk.NewClient(os.Args[2], os.Args[3], sdk.DefaultHTTPClient)

    switch os.Args[1] {
    case "import-datasource":
        if datasources, err = c.GetAllDatasources(); err != nil {
            fmt.Fprintf(os.Stderr, fmt.Sprintf("%s\n", err))
            os.Exit(1)
        }
        filesInDir, err = ioutil.ReadDir(".")
        if err != nil {
            fmt.Fprintf(os.Stderr, fmt.Sprintf("%s\n", err))
        }
        for _, file := range filesInDir {
            if strings.HasSuffix(file.Name(), ".json") {
                if rawDS, err = ioutil.ReadFile(file.Name()); err != nil {
                    fmt.Fprint(os.Stderr, fmt.Sprintf("%s\n", err))
                    continue
                }
                var newDS sdk.Datasource
                if err = json.Unmarshal(rawDS, &newDS); err != nil {
                    fmt.Fprint(os.Stderr, fmt.Sprintf("%s\n", err))
                    continue
                }
                for _, existingDS := range datasources {
                    if existingDS.Name == newDS.Name {
                        c.DeleteDatasource(existingDS.ID)
                        break
                    }
                }
                if status, err = c.CreateDatasource(newDS); err != nil {
                    fmt.Fprint(os.Stderr, fmt.Sprintf("error on importing datasource %s with %s (%s)", newDS.Name, err, *status.Message))
                }
            }
        }
    case "import-dashboard":
        filesInDir, err = ioutil.ReadDir(".")
        if err != nil {
            log.Fatal(err)
        }
        for _, file := range filesInDir {
            if strings.HasSuffix(file.Name(), ".json") {
                if rawBoard, err = ioutil.ReadFile(file.Name()); err != nil {
                    log.Println(err)
                    continue
                }
                var board sdk.Board
                if err = json.Unmarshal(rawBoard, &board); err != nil {
                    log.Println(err)
                    continue
                }
                c.DeleteDashboard(board.UpdateSlug())
                _, err := c.SetDashboard(board, false)
                if err != nil {
                    log.Printf("error on importing dashboard %s", board.Title)
                    continue
                }
            }
        }
    case "backup-datasource":
        if datasources, err = c.GetAllDatasources(); err != nil {
            fmt.Fprintf(os.Stderr, fmt.Sprintf("%s\n", err))
            os.Exit(1)
        }
        for _, ds := range datasources {
            if dsPacked, err = json.Marshal(ds); err != nil {
                fmt.Fprintf(os.Stderr, fmt.Sprintf("%s for %s\n", err, ds.Name))
                continue
            }
            if err = ioutil.WriteFile(fmt.Sprintf("%s.json", slug.Make(ds.Name)), dsPacked, os.FileMode(int(0666))); err != nil {
                fmt.Fprintf(os.Stderr, fmt.Sprintf("%s for %s\n", err, meta.Slug))
            }
        }
    case "backup-dashboard":
        if boardLinks, err = c.SearchDashboards("", false); err != nil {
            fmt.Fprintf(os.Stderr, fmt.Sprintf("%s\n", err))
            os.Exit(1)
        }
        for _, link := range boardLinks {
            if rawBoard, meta, err = c.GetRawDashboard(link.URI); err != nil {
                fmt.Fprintf(os.Stderr, fmt.Sprintf("%s for %s\n", err, link.URI))
                continue
            }
            if err = ioutil.WriteFile(fmt.Sprintf("%s.json", meta.Slug), rawBoard, os.FileMode(int(0666))); err != nil {
                fmt.Fprintf(os.Stderr, fmt.Sprintf("%s for %s\n", err, meta.Slug))
            }
        }
    default:
        fmt.Fprint(os.Stderr, "Usage:  ./grafana-tools <commmand> http://sdk.host:3000 <username:pw | api-key>\n" +
                   "\ncommand: import-datasource, import-dashboard, backup-datasource, or backup-dashboard\n")
        os.Exit(0)
    }


}