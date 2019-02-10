<template>
    <drop class="drop" @drop="handleDrop" @dragover="onDragEnter" @dragleave="onDragLeave" drop-effect="copy">
        <template v-if="!resultPresent">
            <div class="drop-content" v-if="!processing">
                <input autofocus v-model="inputUrl" type="text" @input="onChangedUrl()" placeholder="Enter VK url"/>
                <h1>Drop VK url here</h1>
            </div>
            <spinner v-else class="send-spinner" :line-size="10" :spacing="20" :speed="0.4" size="55" :font-size="20" message="Processing"></spinner>
        </template>
        <div v-else v-bind:class="{ 'result': true, 'success': resultSuccess, 'fail': !resultSuccess }">
            <span>{{inputUrl}}</span>
            <ul v-if="resultSuccess">
                <li v-for="element in resultList" :key="element.url"><a :href="element.url">{{element.title}}</a></li>
            </ul>
            <div v-else>
                {{errorMessage}}
            </div>
            <span class="close" @click="closeResult()">close</span>
        </div>
    </drop>
</template>

<script>
    import {Drop} from 'vue-drag-drop';
    import Spinner from 'vue-simple-spinner'
    import debounce from "lodash/debounce";

    export default {
        name: 'Upload',
        components: {Drop, Spinner},
        data(){
            return {
                inputUrl: '',
                processing: false,
                resultList: [],
                resultSuccess: true,
                errorMessage: ''
            }
        },
        methods: {
            handleDrop(data, event) {
                console.log("handleDrop event", data, event);
                event.preventDefault();
                const url = event.dataTransfer.getData('text/uri-list');
                console.log("handleDrop url=", url);
                this.$data.inputUrl = url;
                this.doWork();
            },
            onDragEnter(){
                if (document.querySelector('.drop h1')) {
                    document.querySelector('.drop h1').style.color = 'white';
                }
                if (document.querySelector('.drop')) {
                    document.querySelector('.drop').style.background = 'rgba(0, 255, 152, 0.37)';
                }
            },
            onDragLeave(){
                this.resetStyle();
            },
            onChangedUrl(){
                this.doWork();
            },
            resetStyle(){
                if (document.querySelector('.drop h1')) {
                    document.querySelector('.drop h1').style.color = 'grey';
                }
                if (document.querySelector('.drop')) {
                    document.querySelector('.drop').style.background = '#f1f1f1';
                }
            },
            doWork(){
                if (this.$data.inputUrl) {
                    this.$data.processing = true;
                    this.$http.put('/process?url=' + this.$data.inputUrl).then(response => {
                        this.$data.processing = false;
                        this.$data.resultSuccess = true;
                        this.$data.errorMessage = '';
                        this.$data.resultList = response.data;
                        this.resetStyle();
                    }, response => {
                        console.error("Error on process url", response);
                        this.$data.processing = false;
                        this.$data.resultSuccess = false;
                        this.$data.errorMessage = 'Error';
                        this.$data.resultList = [];
                        this.resetStyle();
                    })
                }
            },
            closeResult(){
                this.$data.resultList = [];
                this.$data.inputUrl = '';
                this.$data.resultSuccess = true;
                this.$data.errorMessage = '';
                this.resetStyle();
            }
        },
        created() {
            // https://forum-archive.vuejs.org/topic/5174/debounce-replacement-in-vue-2-0
            this.onChangedUrl = debounce(this.onChangedUrl, 700);
        },
        computed:{
            resultPresent(){
                return this.$data.resultList.length > 0
            }
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
        justify-content center
        align-items center
        width 100%
        height 100%
        font-family monospace, serif

        input {
            border-style hidden
            //border-width 0px
            width 100%
            box-sizing: border-box;
            font-size x-large
        }

        .drop {
            display flex
            flex-direction column
            width 90%
            height 90%
            justify-content center

            background #f1f1f1

            h1 {
                position relative
                font-size xx-large
                color grey
                top 40%
                width 100%
                text-align center
            }

            border-width 4px
            border-style dashed

            &-content {
                width: 100%
                height: 100%
            }
        }

        .result {
            display flex
            align-items center
            justify-content center
            flex-direction column
            width 100%
            height 100%

            ul {
                li {
                    font-size x-large
                }
            }


            span.close {
                cursor pointer
                color crimson
            }
        }

        .success {
            animation: colorChangeSuccess 4s;
            @keyframes colorChangeSuccess
            {
                0%   {background: rgba(0, 255, 152, 0.37);}
                100% {background: #f1f1f1;}
            }
        }

        .fail {
            animation: colorChangeFail 4s;
            @keyframes colorChangeFail
            {
                0%   {background: rgba(255, 0, 3, 0.36);}
                100% {background: #f1f1f1;}
            }
        }

    }
</style>
