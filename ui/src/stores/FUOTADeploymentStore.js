import { EventEmitter } from "events";

import Swagger from "swagger-client";

import sessionStore from "./SessionStore";
import {checkStatus, errorHandler } from "./helpers";
import dispatcher from "../dispatcher";


class FUOTADeploymentStore extends EventEmitter {
  constructor() {
    super();
    this.swagger = new Swagger("/swagger/fuotaDeployment.swagger.json", sessionStore.getClientOpts());
  }

  createForDevice(devEUI, fuotaDeployment, callbackFunc) {
    this.swagger.then(client => {
      client.apis.FUOTADeploymentService.Create({
        body: {
          devEUI: devEUI,
          fuotaDeployment: fuotaDeployment,
        };
      })
        .then(checkStatus)
        .then(resp => {
          this.notify("created");
          callbackFunc(resp.ob);
        })
      .catch(errorHandler);
    });
  }

  notify(action) {
    dispatcher.dispatch({
      type: "CREATE_NOTIFICATION",
      notification: {
        type: "success",
        message: "fuota deployment has been " + action,
      },
    });
  }

}

const fuotaDeploymentStore = new FUOTADeploymentStore();
export default fuotaDeploymentStore;

