pipeline {
    agent any

    options {
        skipDefaultCheckout(true)
        disableConcurrentBuilds()
        timeout(time: 45, unit: 'MINUTES')
        buildDiscarder(logRotator(numToKeepStr: '20'))
        timestamps()
    }

    parameters {
        string(name: 'SOURCE_REF', defaultValue: '', trim: true,
            description: 'Required: reviewed full 40-character source commit SHA, reachable in this job SCM')
        string(name: 'EXPECTED_COMMIT', defaultValue: '', trim: true,
            description: 'Required: the same reviewed full SHA; mismatch fails before building')
        booleanParam(name: 'BUILD_ONLY', defaultValue: true,
            description: 'Required true: test, build and push the dev image only; never deploy or update GitOps')
    }

    environment {
        CI = 'true'
        APP_GIT_URL = 'https://github.com/seesaws/aurora-helix-go-execution-engine.git'
        IMAGE_REPOSITORY = '192.168.0.31/aurora-helix-dev/aurora-go-execution'
        HARBOR_LOGIN_URL = 'http://192.168.0.31'
        HARBOR_CREDENTIALS_ID = 'd0278fbe-ed06-43e6-82d3-7c622f896195'
        GO_BUILDER_IMAGE = 'golang:1.26-bookworm@sha256:ee3901c61e423732605a51f1b8157c849ec01194ea0a274213b5202b2c11e7bf'
    }

    stages {
        stage('Validate and checkout exact source') {
            steps {
                script {
                    if (!(params.SOURCE_REF ==~ /[0-9a-f]{40}/)
                            || params.EXPECTED_COMMIT != params.SOURCE_REF
                            || params.BUILD_ONLY != true) {
                        error 'Use equal full SHA parameters and BUILD_ONLY=true; this job has no deployment mode'
                    }
                    def remotes = scm.userRemoteConfigs
                    if (remotes.size() != 1 || remotes[0].url != env.APP_GIT_URL
                            || remotes[0].credentialsId) {
                        error 'Configure this Pipeline-from-SCM job for the fixed public Go HTTPS remote with credentials none'
                    }
                    checkout scmGit(
                        branches: [[name: params.SOURCE_REF]],
                        extensions: [[$class: 'CleanBeforeCheckout']],
                        userRemoteConfigs: [[url: env.APP_GIT_URL]]
                    )
                    env.SOURCE_COMMIT = sh(returnStdout: true, script: 'git rev-parse HEAD').trim()
                    if (env.SOURCE_COMMIT != params.EXPECTED_COMMIT) {
                        error 'Checked-out source does not match EXPECTED_COMMIT'
                    }
                    env.FULL_IMAGE_NAME = "${env.IMAGE_REPOSITORY}:git-${env.SOURCE_COMMIT}-${env.BUILD_NUMBER}"
                    currentBuild.description = "${env.SOURCE_COMMIT}; dev image only; no deployment"
                }
                sh '''
                    set -eu
                    test -z "$(git status --porcelain)"
                    test -f Dockerfile
                    docker version
                    docker buildx version
                    case "$(docker info --format '{{.OSType}}/{{.Architecture}}')" in
                        linux/amd64|linux/x86_64) ;;
                        *) echo 'Use an isolated Linux amd64 Docker worker'; exit 1 ;;
                    esac
                    mkdir -p .jenkins-build
                    sha256sum Dockerfile > .jenkins-build/dockerfile-sha256.txt
                '''
            }
        }

        stage('Go tests and race detector') {
            steps {
                sh '''
                    set -eu
                    docker run --rm --platform linux/amd64 \
                        --cap-drop ALL --security-opt no-new-privileges \
                        -v "$WORKSPACE:/src:ro" -w /src \
                        -e GOTOOLCHAIN=local -e GOFLAGS=-buildvcs=false \
                        -e GOCACHE=/tmp/go-build -e GOMODCACHE=/tmp/go-mod \
                        "$GO_BUILDER_IMAGE" sh -ec \
                        'go mod download && go mod verify && go test ./... && go test -race ./...'
                '''
            }
        }

        stage('Build and verify amd64 image') {
            steps {
                sh '''
                    set -eu
                    docker buildx build --platform linux/amd64 --load --provenance=false \
                        --build-arg SOURCE_DATE_EPOCH=0 -f Dockerfile -t "$FULL_IMAGE_NAME" .
                    test "$(docker image inspect --format '{{.Os}}/{{.Architecture}} {{.Config.User}}' "$FULL_IMAGE_NAME")" = 'linux/amd64 65532:65532'
                    docker image inspect --format '{{.Id}}' "$FULL_IMAGE_NAME" > .jenkins-build/local-image-id.txt
                    docker run --rm --platform linux/amd64 --network none --read-only \
                        --cap-drop ALL --security-opt no-new-privileges \
                        --entrypoint /app/health-probe "$FULL_IMAGE_NAME" process health-probe
                '''
            }
        }

        stage('Publish dev image and record digest') {
            steps {
                withCredentials([usernamePassword(
                    credentialsId: env.HARBOR_CREDENTIALS_ID,
                    usernameVariable: 'HARBOR_USERNAME', passwordVariable: 'HARBOR_PASSWORD'
                )]) {
                    sh '''
                        set +x
                        set -eu
                        DOCKER_CONFIG=$(mktemp -d)
                        export DOCKER_CONFIG
                        trap 'rm -rf -- "$DOCKER_CONFIG"' EXIT HUP INT TERM
                        printf '%s' "$HARBOR_PASSWORD" | docker login "$HARBOR_LOGIN_URL" \
                            -u "$HARBOR_USERNAME" --password-stdin
                        docker push "$FULL_IMAGE_NAME"
                        docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$FULL_IMAGE_NAME" \
                            | awk -v prefix="$IMAGE_REPOSITORY@sha256:" \
                            'index($0,prefix)==1 {print; n++} END {if (n!=1) exit 1}' \
                            > .jenkins-build/image-digest.txt
                        DIGEST=$(cat .jenkins-build/image-digest.txt)
                        printf '%s' "${DIGEST##*@sha256:}" | grep -Eq '^[0-9a-f]{64}$'
                    '''
                }
                script {
                    writeFile file: '.jenkins-build/image-manifest.json', text: groovy.json.JsonOutput.toJson([
                        revision: env.SOURCE_COMMIT,
                        source_ref: params.SOURCE_REF,
                        image: env.FULL_IMAGE_NAME,
                        digest: readFile('.jenkins-build/image-digest.txt').trim(),
                        local_image_id: readFile('.jenkins-build/local-image-id.txt').trim(),
                        dockerfile_sha256: readFile('.jenkins-build/dockerfile-sha256.txt').tokenize()[0],
                        go_builder: env.GO_BUILDER_IMAGE,
                        platform: 'linux/amd64',
                        runtime_user: '65532:65532',
                        build_only: true,
                        build_url: env.BUILD_URL
                    ]) + '\n'
                }
                archiveArtifacts artifacts: '.jenkins-build/*.txt,.jenkins-build/image-manifest.json', fingerprint: true
            }
        }
    }

    post {
        success {
            echo 'Dev image published; use the archived repository@sha256 digest for a separately reviewed deployment'
        }
        always {
            deleteDir()
        }
    }
}
