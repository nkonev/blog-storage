import Vue from 'vue'
import Vuex from 'vuex'

Vue.use(Vuex);

export const GET_UNAUTHORIZED = 'getUser';
export const SET_UNAUTHORIZED = 'setUser';

const store = new Vuex.Store({
    state: {
        unauthorized: false,
    },
    mutations: {
        [SET_UNAUTHORIZED](state, payload) {
            state.unauthorized = payload;
        },
    },
    getters: {
        [GET_UNAUTHORIZED](state) {
            return state.unauthorized;
        },
    },
    actions: {
    }
});

export default store;