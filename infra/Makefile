export TERM=xterm-256color

ENV      ?= lab
TFVARS   ?= env/$(ENV)/terraform.tfvars
BACKEND  ?= env/$(ENV)/backend.hcl
PLAN_OUT ?= create.tfplan

.PHONY: fmt init init-no-backend validate plan apply destroy-plan destroy ci-validate

fmt:
	@terraform fmt -recursive

init:
	@terraform init -upgrade -backend-config=$(BACKEND)

init-no-backend:
	@terraform init -backend=false

validate: init-no-backend
	@terraform validate

plan: init
	@terraform plan -input=false -var-file=$(TFVARS) -out=$(PLAN_OUT)

apply: init
	@terraform apply -input=false -auto-approve $(PLAN_OUT)

destroy-plan: init
	@terraform plan -destroy -input=false -var-file=$(TFVARS) -out=destroy.tfplan

destroy: init
	@terraform apply -input=false destroy.tfplan

ci-validate:
	@terraform fmt -check -recursive
	@terraform init -backend=false
	@terraform validate
