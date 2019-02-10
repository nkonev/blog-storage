import { shallowMount } from '@vue/test-utils'
import App from '@/App.vue'

describe('App.vue', () => {
  it('renders message', () => {
    const msg = 'Drop VK url here';
    const wrapper = shallowMount(App, {
      propsData: {  }
    });
    expect(wrapper.text()).toMatch(msg)
  });

  it('request backend on drop event', (done) => {
    const $http = {
      put: (url)=>{
        expect(url).toContain(`/process?url=`);
        return Promise.resolve({
          data: [
            {title: "Download VK music", url: "http://vk.com/music.zip"},
            {title: "Download VK video", url: "http://vk.com/video.zip"},
          ]
        })
      }
    };

    const wrapper = shallowMount(App, {
      propsData: {  },
      mocks: {$http: $http}
    });

    wrapper.setData({ inputUrl: 'http://vk.com/post123_456' });
    wrapper.vm.doWork();

    // https://vue-test-utils.vuejs.org/guides/testing-async-components.html
    wrapper.vm.$nextTick(() => {
      expect(wrapper.text()).toContain('Download VK music');
      expect(wrapper.text()).toContain('Download VK video');
      done()
    })
  });

  it('emit event', (done)=>{
    const $http = {
      put: (url)=>{
        expect(url).toContain(`/process?url=`);
        done();
        return Promise.resolve({
          data: [ ]
        })
      }
    };

    const wrapper = shallowMount(App, {
      propsData: {  },
      mocks: {$http: $http}
    });

    const el = wrapper.find("input");
    el.element.value = 'http://vk.com/post123_457';
    el.trigger('input');
  });

});