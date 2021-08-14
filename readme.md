# Advanced git-mirror -  Git Repository mirroring

`git-http-backend` is designed to create and serve read-only mirrors of your Git repositories locally or wherever you choose.  A recent GitHub outage reinforces the fact that developers shouldn't be relying on a single remote for hosting code.

Goals:

- [x] 1.<s>Mirroring of Repositories</s>.

- [x] 2.<s>Smart and Dumb HTTP Support</s>.

- [x] 3.<s>No. of Parallel Processes to run parallelly</s>.

- [x] 4.<s>Counter to retry if any Error arises due to network</s>.

- [ ] 5.Gitiles for Repository Viewing.

- [ ] 6.Gerrit for Code Reviewing.



## Get started

Download the latest release from the [https://github.com/Vamsheeth/git-mirror.git](https://github.com/Vamsheeth/git-mirror.git).


____

> Tested only on Linux(Ubuntu).

____

Create `config.toml` similar to:

```toml
[[repo]]
Origin = "https://github.com/Vamsheeth/git-mirror.git"
```
By default it will update the mirror every **15 minutes** and will serve the mirror over HTTP using port **8172**.  You can specify as many repos as you want by having multiple `[[repo]]` sections.

Run `git-http-backend` with the path to the config file:

```bash
$ ./git-http-backend config.toml
2019/05/07 11:08:06 starting web server on :8172   
2019/05/07 11:08:06 updating https://github.com/Vamsheeth/git-mirror.git
2019/05/07 11:08:08 updated https://github.com/Vamsheeth/git-mirror.git
```

Now you can clone from your mirror on the default port of `8172`:

```bash
$ git clone http://localhost:8172/github.com/Vamsheeth/git-mirror.git
Cloning into 'mirror'...
Checking connectivity... done.
```
