# GCP Instance Reset Slack Command

## Usage

### Create `.env.yaml`

```console
$ cp .env.example.yaml .env.yaml
$ vim .env.yaml
```

### Deploy

```console
$ gcloud functions deploy <name> --entry-point ResetInstance --allow-unauthenticated --runtime go113 --security-level secure-always --env-vars-file .env.yaml --trigger-http --project <project-id>
```

## License

[MIT](LICENSE)

## Author

Masahiro Furudate (a.k.a. [178inaba](https://github.com/178inaba))  
<178inaba.git@gmail.com>
