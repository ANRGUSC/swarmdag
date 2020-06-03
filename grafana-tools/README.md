# SwarmDAG Instructions

Install `jq`.

    sudo apt install jq

Run Grafana instance with `--rm` to easily reset.

    docker run -d --rm --name=grafana -p 3000:3000 grafana/grafana

Import from the `swarmdagtemplate/` directory.

    ./importer.sh

Make any changes as you see fit and then export to save.

    ./exporter.sh

The config.sh information is set for a fresh Grafana container.

Originally taken from https://github.com/hagen1778/grafana-import-export. 
Original README below.

# grafana-import-export

Simple scripts for import/export dashboards, datasources and alerts to [Grafana](http://grafana.org/)

Support organizations.

Example was taken from https://gist.github.com/crisidev/bd52bdcc7f029be2f295 

## Dependencies
**[JQ](https://stedolan.github.io/jq/)** - to process .json

## Configuration
Replace **HOST** and **FILE_DIR** variables at `config.sh` with your own.
Also fill **ORGS** array with pairs ORGANIZATION:API_KEY.

## exporter
Run:
```
./exporter.sh
```

Expected output:
```
./exporter.sh
  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                 Dload  Upload   Total   Spent    Left  Speed
100 21102    0 21102    0     0  53000      0 --:--:-- --:--:-- --:--:-- 53020

```

Look for exported .json dashboards and datasources at **FILE_DIR** path

## importer
To import all .json files from **FILE_DIR** to your Grafana:
```
./importer.sh
```

To import only some of them:
```
./importer.sh organization/dashboards/dashboard.json organization/datasources/datasource.json
```

To import all for organization:
```
./importer.sh organization/dashboards/*.json organization/datasources/*.json
```
