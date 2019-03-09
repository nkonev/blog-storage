<template>
    <v-app>
        <v-toolbar app dark class="primary">
            <v-toolbar-title v-text="'Admin App'"></v-toolbar-title>
            <v-spacer/>
            <v-toolbar-items>
                <v-btn flat>Users222</v-btn>
            </v-toolbar-items>
        </v-toolbar>
    </v-app>
</template>

<script>
    import Vue from 'vue'
    import Vuetify from 'vuetify'
    import 'material-design-icons-iconfont/dist/material-design-icons.css'

    Vue.use(Vuetify);

    export default {
        name: 'AdminApp',
    }
</script>

<style lang="stylus">
    @import '~vuetify/src/stylus/main';
</style>

<style lang="stylus" scoped>

</style>
