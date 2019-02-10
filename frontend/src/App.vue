<template>
    <div id="app">
        <ul class="file-list">
            <li v-for="file in files" :key="file.filename"><a :href="file.url">{{file.filename}}</a> <span class="file-list-delete">[x]</span></li>
        </ul>
    </div>
</template>

<script>
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
        }
    }
</script>

<style lang="stylus">
    html, body {
        height: 100%
    }
</style>

<style lang="stylus" scoped>
    html, body {
        height: 100%
    }
    #app {
        display flex
        flex-direction column
        justify-content center
        align-items center
        width 100%
        height 100%
        font-family monospace, serif

        .file-list {
            &-delete {
                cursor: pointer
                color red
            }
        }
    }
</style>
