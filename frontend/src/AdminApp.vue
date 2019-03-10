<template>
    <v-app>
        <v-navigation-drawer
                absolute
                temporary
                v-model="drawer"
                left
                app
        >
            <v-list dense>
                <v-list-tile @click="">
                    <v-list-tile-action>
                        <v-icon>home</v-icon>
                    </v-list-tile-action>
                    <v-list-tile-content>
                        <v-list-tile-title>Home</v-list-tile-title>
                    </v-list-tile-content>
                </v-list-tile>
                <v-list-tile @click="">
                    <v-list-tile-action>
                        <v-icon>contact_mail</v-icon>
                    </v-list-tile-action>
                    <v-list-tile-content>
                        <v-list-tile-title>Contact</v-list-tile-title>
                    </v-list-tile-content>
                </v-list-tile>
            </v-list>
        </v-navigation-drawer>

        <v-toolbar app dark class="primary">
            <v-toolbar-side-icon @click.stop="drawer = !drawer" class="hidden-md-and-up"></v-toolbar-side-icon>
            <v-toolbar-title v-text="'Admin App'"></v-toolbar-title>
            <v-spacer/>
            <v-toolbar-items class="hidden-sm-and-down">
                <v-btn color="success">Success</v-btn>
                <v-btn flat>
                    <v-icon left v-html="'visibility'"></v-icon>
                    Users222
                </v-btn>
            </v-toolbar-items>
        </v-toolbar>
        <v-content>
            <v-container fluid fill-height>
                <v-layout justify-center align-center>
                    <v-flex text-xs-center>
                        <v-btn fab dark small color="primary">
                            <v-icon dark>remove</v-icon>
                        </v-btn>

                        <v-btn fab dark small color="pink">
                            <v-icon dark>favorite</v-icon>
                        </v-btn>

                        <v-btn fab dark color="indigo">
                            <v-icon dark>add</v-icon>
                        </v-btn>

                        <v-btn fab dark color="teal">
                            <v-icon dark>list</v-icon>
                        </v-btn>

                        <v-btn fab dark large color="cyan">
                            <v-icon dark>edit</v-icon>
                        </v-btn>

                        <v-btn fab dark large color="purple">
                            <v-icon dark>android</v-icon>
                        </v-btn>
                    </v-flex>
                </v-layout>
            </v-container>
        </v-content>
        <v-footer app>(C) Cool app</v-footer>
    </v-app>
</template>

<script>
    import Vue from 'vue'
    import Vuetify from 'vuetify'
    import 'material-design-icons-iconfont/dist/material-design-icons.css'

    Vue.use(Vuetify);

    export default {
        data(){
            return {
                drawer: false
            }
        },
        name: 'AdminApp',
    }
</script>

<style lang="stylus">
    @import '~vuetify/src/stylus/main';
</style>

<style lang="stylus" scoped>
    .application {
        font-family: Arial, sans-serif;
        -webkit-font-smoothing: antialiased;
        -moz-osx-font-smoothing: grayscale;

    }
</style>
