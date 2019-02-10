import Vue from 'vue'
import App from './App.vue'
import VueResource from 'vue-resource'
import Notifications from './notifications'
Vue.config.productionTip = false;
Vue.use(VueResource);

function getCookie(name) {
  const value = "; " + document.cookie;
  const parts = value.split("; " + name + "=");
  if (parts.length === 2) return parts.pop().split(";").shift();
}

Vue.http.interceptors.push((request, next) => {

  // https://docs.spring.io/spring-security/site/docs/current/reference/html/csrf.html#csrf-cookie
  const csrfCookieValue = getCookie('XSRF-TOKEN');
  request.headers.set('X-XSRF-TOKEN', csrfCookieValue);

  next((response) => {
    if (!(response.status >= 200 && response.status < 300)) {
      console.error("Unexpected error", response);
      Notifications.error(request.method, request.url, response.status);
    }
  });
});

new Vue({
  render: h => h(App)
}).$mount('#app-container');
