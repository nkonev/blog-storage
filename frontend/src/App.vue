<template>
    <div id="app" class="upload-drag">
        <v-dialog/>
        <div v-if="unauthenticated" class="unauthenticated">
            <h4>Unauthenticated</h4>
            <button @click="refresh()">Refresh</button>
        </div>
        <template v-else-if="showAdminPanel">
            <div class="header">
                <button class="back" @click.prevent="resetShowAdminPanel()">< Back</button>
                <h3>Configure user limits</h3>
            </div>
            <div class="first-list">
                <ul class="user-list">
                    <li v-for="user in users" :key="user.id"><span>#{{user.id}}</span>
                        <span v-if="user.unlimited" class="btn-info" @click="setLimited(user.id, true)">[set limited]</span>
                        <span v-else class="btn-info" @click="setLimited(user.id, false)">[set unlimited]</span>
                    </li>
                </ul>
            </div>
        </template>
        <template v-else>
            <div class="second-list">
                <div v-show="$refs.uploadComponent && $refs.uploadComponent.dropActive" class="drop-active">
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
                                ref="uploadComponent">
                            Select files
                        </file-upload>

                        <template v-if="uploadFiles.length">
                            <button type="button" class="btn btn-success" v-if="!$refs.uploadComponent || !$refs.uploadComponent.active" @click.prevent="$refs.uploadComponent.active = true">
                                Start Upload
                            </button>
                            <button type="button" class="btn btn-danger" v-else @click.prevent="$refs.uploadComponent.active = false">
                                Stop Upload
                            </button>

                            <button type="button" class="btn btn-danger" @click.prevent="reset()">
                                Reset
                            </button>
                        </template>

                        <a href="/" class="tab" target="_blank">Open tab</a>
                        <button class="tab" v-if="admin" @click.prevent="setShowAdminPanel()">Admin panel</button>
                    </div>

                    <div class="limits">Used: {{ bucketUsed | formatSize}}; Available: {{bucketAvailable | formatSize}}</div>
                </div>

                <ul v-if="uploadFiles.length">
                    <li v-for="(file, index) in uploadFiles" :key="file.id">
                        <span>{{file.name}}</span>
                        <span>[{{file.size | formatSize}}]</span><span v-if="file.error || file.success || file.active"> -</span>
                        <span v-if="file.error">{{file.error}}</span>
                        <span v-else-if="file.success">success</span>
                        <span v-else-if="file.active">uploading</span>
                        <span v-else></span>
                        <span class="progress" v-if="file.active || file.progress !== '0.00'">
                            {{file.progress}}%
                        </span>
                        <span class="btn-delete" @click.prevent="deleteUpload(file.name, index)" v-if="!$refs.uploadComponent || !$refs.uploadComponent.active">[x]</span>
                    </li>
                </ul>
            </div>

            <hr/>

            <div class="first-list">
                <ul class="file-list">
                    <li v-for="file in files" :key="file.id"><a :href="file.url" target="_blank">{{file.filename}}</a>
                        <span>[{{file.size | formatSize}}]</span>
                        <template v-if="file.publicUrl">
                            <span class="btn-info" @click.prevent="unshareFile(file.id)">[unshare]</span>
                        </template>
                        <template v-else>
                            <span class="btn-info" @click.prevent="shareFile(file.id)">[share]</span>
                        </template>
                        <span class="btn-info" @click.prevent="infoFile(file)">[i]</span>
                        <span class="btn-info" @click.prevent="renameFile(file)">[r]</span>
                        <span class="btn-delete" @click.prevent="deleteFile(file.id, file.filename)">[x]</span>
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
    import Notifications from "./notifications"

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
                bucketAvailable: 0,
                admin: false,
                showAdminPanel: false,
                users: []
            }
        },
        filters: {
            formatSize: formatSize
        },
        methods: {
            setLimited(id, limited){
                this.$http.patch('/users?userId='+id+'&limited='+limited).then(value => {
                    this.getUsers();
                }, reason => {
                    console.error("error during patch user");
                }).then(value => {

                }, reason => {
                    console.error("error during update users after patch");
                })
            },
            setShowAdminPanel() {
                this.showAdminPanel = true;
                this.getUsers();
            },
            resetShowAdminPanel() {
                this.showAdminPanel = false;
                this.$data.users = []
            },
            getUsers(){
                this.$http.get('/users').then(value => {
                    this.$data.users = value.body.users;
                }, reason => {
                    console.error("error during get users");
                })
            },
            deleteUpload(filename, index){
                console.log("deleting " + filename);

                this.uploadFiles.splice(index, 1)
            },
            doDelete(fileId){
                this.$http.delete('/delete/'+fileId).then(value => {
                    this.ls();
                }, reason => {
                    console.error("error during deleting file");
                });
            },
            deleteFile(fileId, fileName) {
                this.$modal.show(DIALOG, {
                    title: 'Delete confirmation',
                    text: 'Do you want to delete this file "' + fileName +'" ?',
                    buttons: [
                        {
                            title: 'No',
                            default: true,
                            handler: () => {
                                this.$modal.hide(DIALOG)
                            }
                        },
                        {
                            title: 'Yes',
                            handler: () => {
                                this.doDelete(fileId);
                                this.$modal.hide(DIALOG)
                            }
                        },
                    ]
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
                    this.admin = value.data.admin;
                }, reason => {
                    console.error("error during get bucket stat");
                }))
            },
            refresh() {
                this.ls();
            },
            reset(){
                console.log("resetting");
                this.$refs.uploadComponent.clear();
                this.ls();
            },
            shareFile(fileId){
                this.$http.put('/publish/'+fileId).then(value => {
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
            unshareFile(fileId){
                this.$http.delete('/publish/'+fileId).then(value => {
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
                let dto = {newname: file.filename};

                this.$modal.show(DIALOG, {
                    title: 'Rename "' + file.filename + '"',
                    component: Vue.component('rename-component', {
                        data: function () {
                            return {
                                dto: dto
                            }
                        },
                        template: `<div style="display: flex">
                                        <input         v-bind:value="dto.newname"
                                                       v-on:input="dto.newname = $event.target.value"
                                                       style="width:100%;"
                                        ></input>
                                   </div>`
                    }),
                    buttons: [
                        {
                            title: 'Ok',
                            default: true,
                            handler: () => {
                                this.$http.post('/rename/' + file.id, dto).then(value => {
                                    this.$modal.hide(DIALOG);
                                    this.ls();
                                }, reason => {
                                    console.error("error during renaming file");
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
        mounted () {
            // https://gist.github.com/pbojinov/8965299
            const bindEvent = (element, eventName, eventHandler) => {
                if (element.addEventListener) {
                    element.addEventListener(eventName, eventHandler, false);
                } else if (element.attachEvent) {
                    element.attachEvent('on' + eventName, eventHandler);
                }
            };

            const that = this;
            // Listen to messages from parent window
            bindEvent(window, 'message', function (e) {
                if (e.data) {
                    console.log("Event from parent:", e.data);
                    if ('login' == e.data || 'logout' == e.data) {
                        that.refresh();
                    }
                }
            });

            this.$watch(
                () => {
                    return this.$refs.uploadComponent.uploaded
                },
                (val) => {
                    if (val) { // uploaded == true

                        let allSuccess = true;
                        for (let file of this.uploadFiles) {
                            // check status of each file in uploadFiles
                            allSuccess = allSuccess && file.success;
                        }

                        if (allSuccess && this.uploadFiles.length > 0) {
                            // all ok
                            this.reset();
                            Notifications.info("Uploading successfully finished");
                        } else {
                            // was errors
                            Notifications.simpleError("Uploading finished with errors");
                        }
                    }
                }
            )
        }
    }
</script>

<style lang="stylus">
    html, body {
        height: 100%
        width 100%
        display flex
    }

    ul {
        margin 0 0
        padding-left: 0
        li {
            margin 0.4em 0
        }
    }

    .first-list {
        width 100%
        .file-list {
            width 100%
        }
        .user-list {
            width 100%
        }
    }

    .second-list {
        width 100%
    }

    .btn-select {
        color white
        background #00ff77
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
        background #0041ff
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


        hr {
            width 100%
        }

        .header {
            display flex
            justify-content space-between
            align-items: center
            //margin-bottom 0.6em

            .btn {
                margin 1px
            }

            .back {
                margin-right 0.6em
                margin-left 0.6em
            }

            .buttons {
                display flex
                justify-content start
                align-items: center

                a.tab {
                    margin-left 1em
                    margin-right 1em
                }

                @media screen and (min-width: 1000px) {
                    a.tab {
                        display none
                    }
                }
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
