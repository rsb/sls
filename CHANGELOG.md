# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The date format is YYYY-MM-DD
## [Unreleased]
### Fixed
- Prefix OrgUnit had an infinite loop.


## [0.1.3] - 2021-09-13
### Fixed
- renamed lambda trigger from `cognito` to `cog`
- replaced error call with failure

## [0.1.2] - 2021-09-12
### Added
- methods to service to create features map by walking the filesystem  
  looking for lambdas

## [0.1.1] - 2021-09-12
### Added
- Infra and Terraform to the service layout
- DefaultServiceLayout constructor

## [0.1.0] - 2021-09-12
initial release to kick off development. Ideas a basic and will code will change
as I find new patterns

### Added
- ServiceLayout a collection of directories that define a microservice
- Service defines the data and behavior a microservice needs for deployment
- LambdaTrigger a type to describe the aws lambda event trigger
- Region type used to define aws regions