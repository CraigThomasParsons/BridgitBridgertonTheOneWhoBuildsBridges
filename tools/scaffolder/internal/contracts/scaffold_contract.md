# CONTRACT: scaffold

id: job_123

## INPUT
tree_file: scaffold.tree

## TARGET
root: /home/me/Code

## ACTION
materialize filesystem

## VERIFY
- directories exist
- files exist