package scaffold

// defaultExtensions are deployed to ~/.banish/ext/ on init.
// Each key is a filename, each value is the .bsh content.
var defaultExtensions = map[string]string{

	"git.bsh": `!extension git v:1.0

!verb gs
!expand exec git status --short
!help "Git status short format"

!verb ga
!expand exec git add .
!help "Git add all files"

!verb gc
!expand exec git commit
!help "Git commit"

!verb gp
!expand exec git push
!help "Git push to remote"

!verb gl
!expand exec git log --oneline -10
!help "Git log last 10 commits"

!verb gd
!expand exec git diff
!help "Git diff unstaged changes"

!verb gb
!expand exec git branch
!help "List git branches"

!filter git-status
!match git status
!compact "grep -v '^ *$' | grep -v 'use .git' | grep -v 'no changes added' | grep -v 'nothing added' | sed '/^$/d'"

!filter git-log
!match git log
!compact "grep -v '^Author:' | grep -v '^Date:' | grep -v '^Merge:' | grep -v '^ *$' | sed 's/^commit \(.\{7\}\).*/\1/' | sed 's/^    //' | head -50"

!filter git-diff
!match git diff
!compact "grep -v '^index ' | grep -v '^diff --git' | grep -v '^--- a/' | sed 's/^+++ b/--- /' | head -80"

!filter git-branch
!match git branch
!compact "sed 's/^  //' | grep -v '^$'"
`,

	"docker.bsh": `!extension docker v:1.0

!verb dps
!expand exec docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
!help "List running containers (compact)"

!verb dlogs
!args name
!expand exec docker logs --tail 30 {name}
!help "Last 30 log lines from a container"

!verb dimages
!expand exec docker images --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
!help "List images (compact)"

!filter docker-build
!match docker build
!compact "grep -v '^Sending build' | grep -v '^ ---> [a-f0-9]' | grep -v '^Removing intermediate' | grep -v '^ ---> Running' | grep -v '^Step [0-9]' | head -40"

!filter docker-logs
!match docker logs
!compact "tail -30"

!filter docker-compose
!match docker compose up
!compact "grep -v '^Pulling ' | grep -v '^Creating ' | grep -v '^Attaching' | grep -v '^ *$' | head -30"
`,

	"kube.bsh": `!extension kube v:1.0

!verb kpods
!args ns
!expand exec kubectl get pods {ns} -o wide
!help "List pods (wide format)"

!verb klogs
!args pod
!expand exec kubectl logs --tail 40 {pod}
!help "Last 40 log lines from a pod"

!verb kns
!expand exec kubectl get namespaces
!help "List namespaces"

!verb ksvc
!args ns
!expand exec kubectl get svc {ns}
!help "List services"

!verb kdesc
!args resource
!expand exec kubectl describe {resource}
!help "Describe a resource"

!filter kubectl-describe
!match kubectl describe
!compact "grep -v '^ *$' | grep -v 'Annotations:.*<none>' | grep -v 'Labels:.*<none>' | head -60"

!filter kubectl-logs
!match kubectl logs
!compact "tail -40"

!filter kubectl-get
!match kubectl get
!compact "cut -c1-120 | head -50"
`,

	"cloud.bsh": `!extension cloud v:1.0

!verb s3ls
!args path
!expand exec aws s3 ls {path}
!help "List S3 buckets or objects"

!verb ec2ls
!expand exec aws ec2 describe-instances --query "Reservations[].Instances[].[InstanceId,State.Name,InstanceType,PrivateIpAddress]" --output table
!help "List EC2 instances (compact table)"

!verb tfplan
!expand exec terraform plan -no-color
!help "Terraform plan (no color codes)"

!verb tfapply
!expand exec terraform apply -auto-approve -no-color
!help "Terraform apply (auto-approve, no color)"

!filter terraform-plan
!match terraform plan
!compact "grep -v 'Refreshing state' | grep -v 'Acquiring state lock' | grep -v 'Releasing state lock' | grep -v '^ *$' | head -60"

!filter terraform-apply
!match terraform apply
!compact "grep -v 'Refreshing state' | grep -v 'Acquiring state lock' | grep -v '^ *$' | head -60"

!filter aws-s3
!match aws s3
!compact "grep -v '^ *$' | head -50"

!filter aws-ec2
!match aws ec2
!compact "cut -c1-120 | head -60"
`,

	"node.bsh": `!extension node v:1.0

!verb ni
!args pkg
!expand exec npm install {pkg}
!help "npm install"

!verb nr
!args script
!expand exec npm run {script}
!help "npm run script"

!verb yi
!args pkg
!expand exec yarn install {pkg}
!help "yarn install"

!verb pi
!args pkg
!expand exec pnpm install {pkg}
!help "pnpm install"

!filter npm-install
!match npm install
!compact "grep -v '^npm warn' | grep -v '^npm notice' | grep -v '^ *$' | tail -5"

!filter npm-run
!match npm run
!compact "grep -v '^ *$' | grep -v '^>' | grep -v '^npm warn' | head -40"

!filter yarn-install
!match yarn install
!compact "grep -v '^info ' | grep -v '^warning ' | grep -v '^ *$' | tail -5"

!filter pnpm-install
!match pnpm install
!compact "grep -v '^Progress:' | grep -v '^\\.\\.' | grep -v '^ *$' | tail -5"
`,

	"python.bsh": `!extension python v:1.0

!verb ptest
!args path
!expand exec python3 -m pytest {path}
!help "Run pytest"

!verb pipi
!args pkg
!expand exec pip3 install {pkg}
!help "pip install package"

!filter pytest
!match pytest
!compact "grep -v '^=\+' | grep -v '^platform ' | grep -v '^cachedir' | grep -v '^rootdir' | grep -v '^plugins' | grep -v '^collected ' | grep -v '^ *$' | head -50"

!filter pip-install
!match pip install
!compact "grep -v 'Requirement already' | grep -v 'Downloading ' | grep -v '  Using cached' | grep -v '^ *$' | tail -5"
`,

	"rust.bsh": `!extension rust v:1.0

!verb cb
!expand exec cargo build
!help "Cargo build"

!verb ct
!args target
!expand exec cargo test {target}
!help "Cargo test"

!verb cc
!expand exec cargo clippy --all-targets
!help "Cargo clippy"

!verb cr
!args target
!expand exec cargo run {target}
!help "Cargo run"

!filter cargo-build
!match cargo build
!compact "grep -v '^ *Compiling ' | grep -v '^ *Downloading ' | grep -v '^ *Downloaded ' | grep -v '^ *$' | tail -5"

!filter cargo-test
!match cargo test
!compact "grep -v '^ *Compiling ' | grep -v '^ *Downloading ' | grep -v '^ *$' | grep -v '^running ' | head -50"

!filter cargo-clippy
!match cargo clippy
!compact "grep -v '^ *Compiling ' | grep -v '^ *Checking ' | grep -v '^ *$' | head -40"
`,

	"java.bsh": `!extension java v:1.0

!verb mvn
!args goal
!expand exec mvn {goal} -q
!help "Run Maven (quiet mode)"

!verb gw
!args task
!expand exec ./gradlew {task} --console=plain
!help "Run Gradle wrapper (plain console)"

!filter mvn-build
!match mvn
!compact "grep -v '^\[INFO\] ---' | grep -v '^\[INFO\] Building ' | grep -v '^\[INFO\] Downloading' | grep -v '^\[INFO\] Downloaded' | grep -v '^ *$' | head -40"

!filter gradle-build
!match gradlew
!compact "grep -v '^ *$' | grep -v '^Download ' | grep -v '^> Task ' | grep -v '^Deprecated' | head -40"
`,
}
