import { shallowMount } from '@vue/test-utils'
import App from '@/App.vue'

jest.mock('@/notifications', () => () => ({
  error(m, b, s) {
    console.error("error: "+ m + ", " + b +", "+ s)
  },
  simpleError(title, message) {
    console.error("error: title="+ title+ ", message=" + message)
  },
  info(title, message){
    console.info("info: title="+ title+ ", message=" + message)
  }
}));

describe('App.vue', () => {
  it('true', () => {
    expect("true").toMatch("true")
  });
});