<template>
    <div id="app" class="upload-drag">
        <v-dialog/>
        <div v-if="unauthenticated" class="unauthenticated">
            <h4>Unauthenticated</h4>
            <button @click="refresh()">Refresh</button>
        </div>
        <template v-else>
            <div class="second-list">
                <div v-show="$refs.upload && $refs.upload.dropActive" class="drop-active">
                    <h3>Drop files to upload</h3>
                </div>

                <div class="header">
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

                            <button type="button" class="btn btn-danger" @click.prevent="reset()">
                                Reset
                            </button>
                        </template>
                    </div>

                    <div class="limits">Used: {{ bucketUsed | formatSize}}; Available: {{bucketAvailable | formatSize}}</div>
                </div>

                <ul v-if="uploadFiles.length">
                    <li v-for="(file, index) in uploadFiles" :key="file.id">
                        <span>{{file.name}}</span> -
                        <span>{{file.size | formatSize}}</span><span v-if="file.error || file.success || file.active"> -</span>
                        <span v-if="file.error">{{file.error}}</span>
                        <span v-else-if="file.success">success</span>
                        <span v-else-if="file.active">active</span>
                        <span v-else></span>
                        <span class="progress" v-if="file.active || file.progress !== '0.00'">
                            {{file.progress}}%
                        </span>
                        <span class="btn-delete" @click.prevent="deleteUpload(file.name, index)" v-if="!$refs.upload || !$refs.upload.active">[x]</span>
                    </li>
                </ul>
            </div>

            <hr/>

            <div class="first-list">
                <ul class="file-list">
                    <li v-for="file in files" :key="file.id"><a :href="file.url" target="_blank">{{file.filename}}</a> -
                        <span>{{file.size | formatSize}}</span>
                        <template v-if="file.publicUrl">
                            <span class="btn-info" @click.prevent="unshareFile(file.id)">[unshare]</span>
                        </template>
                        <template v-else>
                            <span class="btn-info" @click.prevent="shareFile(file.id)">[share]</span>
                        </template>
                        <span class="btn-info" @click.prevent="infoFile(file)">[i]</span>
                        <span class="btn-info" @click.prevent="renameFile(file)">[r]</span>
                        <span class="btn-delete" @click.prevent="deleteFile(file.id)">[x]</span>
                    </li>
                </ul>
            </div>

        </template>
    </div>
</template>

<script>
    import Vue from 'vue'
    import FileUpload from 'vue-upload-component'
    import store, {GET_UNAUTHENTICATED} from "./store"
    import {mapGetters} from 'vuex'
    import vmodal from 'vue-js-modal'

    const DIALOG = "dialog";

    Vue.use(vmodal, { dialog: true });

    const formatSize = (size) => {
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
    };

    export default {
        name: 'App',
        data(){
            return {
                files: [],
                uploadFiles: [],
                bucketUsed: 0,
                bucketAvailable: 0
            }
        },
        filters: {
            formatSize: formatSize
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
                }).then(this.$http.get('/limits').then(value => {
                    this.bucketUsed = value.data.used;
                    this.bucketAvailable = value.data.available;
                }, reason => {
                    console.error("error during get bucket stat");
                }))
            },
            refresh() {
                this.ls();
            },
            reset(){
                console.log("resetting");
                this.$refs.upload.clear();
                this.ls();
            },
            shareFile(file){
                this.$http.put('/publish/'+encodeURIComponent(file)).then(value => {
                    const url = value.data.url;
                    console.log("Got public url", url);
                    this.ls();

                    this.$modal.show(DIALOG, {
                        title: 'Published',
                        text: `<a href="${url}" target="_blank">Public link</a>`,
                        buttons: [
                            {
                                title: 'Close',
                                default: true,
                                handler: () => {
                                    this.$modal.hide(DIALOG)
                                }
                            },
                        ],
                    })
                }, reason => {
                    console.error("error during sharing file");
                })
            },
            unshareFile(file){
                this.$http.delete('/publish/'+encodeURIComponent(file)).then(value => {
                    this.ls();
                }, reason => {
                    console.error("error during unsharing file");
                })
            },
            infoFile(file){
                this.$modal.show(DIALOG, {
                    title: 'Info',
                    text: `<p>${file.filename}</p>
                           <p>${ formatSize(file.size)}</p>
                           ${file.publicUrl ? '<p><a href="' +file.publicUrl +'" target="_blank">Public link</a></p>' : ''}
 `,
                    buttons: [
                        {
                            title: 'Close',
                            default: true,
                            handler: () => {
                                this.$modal.hide(DIALOG)
                            }
                        },
                    ],
                })
            },
            renameFile(file){
                this.$modal.show(DIALOG, {
                    title: 'Rename',
                    text: `<p>${file.filename}</p>`,
                    buttons: [
                        {
                            title: 'Ok',
                            default: true,
                            handler: () => {
                                this.$http.post('/rename/'+file.id, {newname: ''+new Date().getTime()}).then(value => {
                                    this.$modal.hide(DIALOG);
                                    this.ls();
                                }, reason => {
                                    console.error("error during unsharing file");
                                })
                            }
                        },
                        {
                            title: 'Close',
                            handler: () => {
                                this.$modal.hide(DIALOG)
                            }
                        },
                    ],
                })
            }
        },
        watch: {
            uploadFiles: {
                handler: function (val, oldVal) {
                    let allSuccess = true;
                    for (let file of val) {
                        allSuccess = allSuccess && file.success;
                    }
                    if (allSuccess && this.uploadFiles.length > 0) {
                        this.reset();
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
        },
        computed: {
            ...mapGetters({unauthenticated: GET_UNAUTHENTICATED}), // unauthorized is here, 'GET_UNAUTHORIZED' -- in store.js
        },
        store,
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

    .btn-select:hover {
        color white
        background-color #003eff
        border-radius 2px
        opacity: 0.8
        z-index 1000
        filter brightness(2)
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

        .header {
            display flex
            justify-content space-between
            align-items: center

            .btn {
                margin 1px
            }

            .buttons {
                display flex
                justify-content start
                align-items: center
            }

            .limits {
                margin 4px
            }
        }

        .unauthenticated {
            display flex
            flex-direction column
            align-items center
            justify-content center
            height 100%
            width 100%
            h4 {
                margin 0 0 0.8em 0
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

    .btn-info {
        cursor: pointer
        color blue
    }

</style>
