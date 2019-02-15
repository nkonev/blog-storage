<template>
    <div id="app" class="upload-drag">

        <div class="second-list">
            <div v-show="$refs.upload && $refs.upload.dropActive" class="drop-active">
                <h3>Drop files to upload</h3>
            </div>

            <div class="buttons">
                <file-upload
                        class="btn btn-select"
                        post-action="/upload"
                        :multiple="true"
                        :drop="true"
                        :drop-directory="true"
                        v-model="uploadFiles"
                        ref="upload">
                    Select files
                </file-upload>

                <template v-if="uploadFiles.length">
                    <button type="button" class="btn btn-success" v-if="!$refs.upload || !$refs.upload.active" @click.prevent="$refs.upload.active = true">
                        Start Upload
                    </button>
                    <button type="button" class="btn btn-danger" v-else @click.prevent="$refs.upload.active = false">
                        Stop Upload
                    </button>

                    <button type="button" class="btn btn-danger" @click.prevent="$refs.upload.clear()">
                        Reset
                    </button>
                </template>
            </div>

            <ul v-if="uploadFiles.length">
                <li v-for="(file, index) in uploadFiles" :key="file.id">
                    <span>{{file.name}}</span> -
                    <span>{{file.size | formatSize}}</span><span v-if="file.error || file.success || file.active"> -</span>
                    <span v-if="file.error">{{file.error}}</span>
                    <span v-else-if="file.success">success</span>
                    <span v-else-if="file.active">active</span>
                    <span v-else></span>
                    <span class="btn-delete" @click.prevent="deleteUpload(file.name, index)" v-if="!$refs.upload || !$refs.upload.active">[x]</span>
                </li>
            </ul>
        </div>

        <hr/>

        <div class="first-list">
            <ul class="file-list">
                <li v-for="file in files" :key="file.filename"><a :href="file.url" target="_blank">{{file.filename}}</a> - <span>{{file.size | formatSize}}</span> <span class="btn-delete" @click.prevent="deleteFile(file.filename)">[x]</span></li>
            </ul>
        </div>
    </div>
</template>

<script>
    import Vue from 'vue'
    import FileUpload from 'vue-upload-component'

    Vue.filter('formatSize', function (size) {
        if (size > 1024 * 1024 * 1024 * 1024) {
            return (size / 1024 / 1024 / 1024 / 1024).toFixed(2) + ' TB'
        } else if (size > 1024 * 1024 * 1024) {
            return (size / 1024 / 1024 / 1024).toFixed(2) + ' GB'
        } else if (size > 1024 * 1024) {
            return (size / 1024 / 1024).toFixed(2) + ' MB'
        } else if (size > 1024) {
            return (size / 1024).toFixed(2) + ' KB'
        }
        return size.toString() + ' B'
    });

    export default {
        name: 'App',
        data(){
            return {
                files: [],
                uploadFiles: []
            }
        },
        methods: {
            deleteUpload(filename, index){
                console.log("deleting " + filename);

                this.uploadFiles.splice(index, 1)
            },
            deleteFile(filename) {
                this.$http.delete('/delete/'+encodeURIComponent(filename)).then(value => {
                    this.ls();
                }, reason => {
                    console.error("error during deleting file");
                })
            },
            ls(){
                this.$http.get('/ls').then(value => {
                    this.$data.files = value.body.files;
                }, reason => {
                    console.error("error during get files");
                })
            }
        },
        watch: {
            uploadFiles: {
                handler: function (val, oldVal) {
                    let allSuccess = true;
                    for (file of val) {
                        allSuccess = allSuccess && file.success;
                    }
                    if (allSuccess) {
                        console.log('all files success', allSuccess);
                        this.$refs.upload.clear();
                        this.ls();
                    }
                },
                deep: true
            }
        },
        created(){
            this.ls();
        },
        components:{
            FileUpload,
        }
    }
</script>

<style lang="stylus">
    html, body {
        height: 100%
    }

    .first-list {
        width 100%
    }

    .second-list {
        width 100%
    }

    .btn-select {
        color white
        background blue
        border-radius 2px
        padding 3px 3px

        input {
            display: none
        }

        label {
            cursor pointer
        }
    }

</style>

<style lang="stylus" scoped>
    html, body {
        height: 100%
    }
    #app {
        display flex
        flex-direction column
        justify-content start
        align-items center
        width 100%
        height 98%
        font-family monospace, serif
        overflow-y auto
        overflow-x hidden


        .file-list {
            width 100%
        }

        hr {
            width 100%
        }

        .buttons {
            display flex
            justify-content start
            align-items: center

            .btn {
                margin 1px
            }
        }
    }

    .upload-drag {
        .drop-active {
            top: 0;
            bottom: 0;
            right: 0;
            left: 0;
            position: fixed;
            z-index: 9999;
            opacity: .6;
            text-align: center;
            background: #000;
        }

        .drop-active h3 {
            margin: -.5em 0 0;
            position: absolute;
            top: 50%;
            left: 0;
            right: 0;
            -webkit-transform: translateY(-50%);
            -ms-transform: translateY(-50%);
            transform: translateY(-50%);
            font-size: 40px;
            color: #fff;
            padding: 0;
        }
    }

    .btn-delete {
        cursor: pointer
        color red
    }

</style>
