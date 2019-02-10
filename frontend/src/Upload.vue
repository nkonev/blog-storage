<template>
    <div class="example-drag">
        <div class="upload">
            <ul v-if="files.length">
                <li v-for="(file, index) in files" :key="file.id">
                    <span>{{file.name}}</span> -
                    <span>{{file.size | formatSize}}</span><span v-if="file.error || file.success || file.active"> -</span>
                    <span v-if="file.error">{{file.error}}</span>
                    <span v-else-if="file.success">success</span>
                    <span v-else-if="file.active">active</span>
                    <span v-else></span>
                </li>
            </ul>

            <div v-show="$refs.upload && $refs.upload.dropActive" class="drop-active">
                <h3>Drop files to upload</h3>
            </div>

            <div class="example-btn">
                <file-upload
                        class="btn btn-primary"
                        post-action="/upload/post"
                        :multiple="true"
                        :drop="true"
                        :drop-directory="true"
                        v-model="files"
                        ref="upload">
                    <i class="fa fa-plus"></i>
                    Upload files
                </file-upload>
                <template v-if="files.length">
                <button type="button" class="btn btn-success" v-if="!$refs.upload || !$refs.upload.active" @click.prevent="$refs.upload.active = true">
                    <i class="fa fa-arrow-up" aria-hidden="true"></i>
                    Start Upload
                </button>
                <button type="button" class="btn btn-danger"  v-else @click.prevent="$refs.upload.active = false">
                    <i class="fa fa-stop" aria-hidden="true"></i>
                    Stop Upload
                </button>
                </template>
            </div>
        </div>
    </div>
</template>
<style>
    .example-drag{
        /*background: antiquewhite;*/
    }
    .example-drag label.btn {
        margin-bottom: 0;
        margin-right: 1rem;
    }
    .example-drag .drop-active {
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
    .example-drag .drop-active h3 {
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

    .example-drag label.btn {

        margin-bottom: 0;
        margin-right: 1rem;

    }
    .btn-group-lg > .btn, .btn-lg {

        padding: .5rem 1rem;
        font-size: 1.25rem;
        line-height: 1.5;
        border-radius: .3rem;

    }
    .btn-primary {

        color: #fff;
        background-color: #007bff;
        border-color: #007bff;

    }
    .btn {
        display: inline-block;
        font-weight: 400;
        text-align: center;
        white-space: nowrap;
        vertical-align: middle;
        -webkit-user-select: none;
        -moz-user-select: none;
        -ms-user-select: none;
        user-select: none;
        border: 1px solid transparent;
        border-top-color: transparent;
        border-right-color: transparent;
        border-bottom-color: transparent;
        border-left-color: transparent;
        padding: .5rem .75rem;
        font-size: 1rem;
        line-height: 1.25;
        border-radius: .25rem;
        transition: all .15s ease-in-out;
        cursor: pointer;
    }
</style>

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
        components: {
            FileUpload,
        },
        data() {
            return {
                files: [],
            }
        }
    }
</script>