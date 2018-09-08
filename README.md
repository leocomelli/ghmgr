# github migrator

A basic tool to help you migrate your repositories from an instance to another (not only change ownership).

## configuration

```yaml
---
source: 
  url: https://github.instance1.mycompany.com/api/v3/
  token: s3cr3t
  organization: leonardo-comelli
  ignore:
    - repo1
    - repoN
  content:
    path: README.md
    message: This repository was migrated to MyCompany Github automatically. [Click here]({{url}})
  archive: true
target:
  url: https://github.instance2.mycompany.com/api/v3/
  token: s3cr3t
  organization: lcomelli
git:
  clone_path: /tmp
  remote_name: new
  ctr_file: /Users/leocomelli/.ssh/id_rsa
  commit_author: Leonardo Comelli
  commit_email: leonardo.comelli@mycompany.com
```

# Flow

1. List repositories by organization in the `source`;
2. Apply filter to `ignore` some repos;
3. Create a new repository on `target`;
4. Clone repository using ssh credentials (`clone_path`);
5. Add a new remote (`remote_name`);
6. Push the repository files to new remote (`target`);
7. Add a new line on top of `content.path` with `message`;
8. Edit the `source` repository to archived.