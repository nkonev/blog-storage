<template>
    <div id="app">
        <div class="first-list">
            <ul class="file-list">
                <li v-for="file in files" :key="file.filename"><a :href="file.url">{{file.filename}}</a> <span class="file-list-delete">[x]</span></li>
            </ul>
        </div>
        <div class="second-list">
            <Upload/>
        </div>
    </div>
</template>

<script>
    import Upload from "@/Upload"
    export default {
        name: 'App',
        data(){
            return {
                files: []
            }
        },
        methods: {},
        created(){
            this.$http.get('/ls').then(value => {
                this.$data.files = value.body.files;
            }, reason => {
                console.error("error during get files");
            })
        },
        components:{
            Upload
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
        margin-bottom 0.5em
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
        height 100%
        font-family monospace, serif
        overflow-y auto
        overflow-x hidden
        margin-top 0.5em
        margin-bottom 0.5em

        .file-list {
            width 100%
            &-delete {
                cursor: pointer
                color red
            }
        }
    }
</style>
